// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package archiver

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/isolatedclient"
	"github.com/luci/luci-go/common/isolated"
)

// Future is the future of a file pushed to be hashed.
type Future interface {
	// WaitForHashed hangs until the item hash is known.
	WaitForHashed()
	// Error returns any error that occured for this item if any.
	Error() error
	// Digest returns the calculated digest once calculated, empty otherwise.
	Digest() isolated.HexDigest
}

// Archiver is an high level interface to an isolatedclient.IsolateServer.
type Archiver interface {
	io.Closer
	common.Cancelable
	Push(displayName string, src io.ReadSeeker) Future
	PushFile(path string) Future
	Stats() *Stats
}

// UploadStat is the statistic for a single upload.
type UploadStat struct {
	Duration time.Duration
	Size     int64
}

// Statistics from the Archiver.
type Stats struct {
	Hits   []int64
	Misses []int64
	Pushed []*UploadStat
}

func (s *Stats) TotalHits() int64 {
	out := int64(0)
	for _, i := range s.Hits {
		out += i
	}
	return out
}

func (s *Stats) TotalMisses() int64 {
	out := int64(0)
	for _, i := range s.Misses {
		out += i
	}
	return out
}

func (s *Stats) TotalPushed() int64 {
	out := int64(0)
	for _, i := range s.Pushed {
		out += i.Size
	}
	return out
}

func (s *Stats) deepCopy() *Stats {
	out := &Stats{}
	out.Hits = make([]int64, len(s.Hits))
	copy(out.Hits, s.Hits)
	out.Misses = make([]int64, len(s.Misses))
	copy(out.Misses, s.Misses)
	out.Pushed = make([]*UploadStat, len(s.Pushed))
	copy(out.Pushed, s.Pushed)
	return out
}

// New returns a thread-safe Archiver instance.
//
// TODO(maruel): Cache hashes and server cache presence.
func New(is isolatedclient.IsolateServer) Archiver {
	a := &archiver{
		canceler:              common.NewCanceler(),
		is:                    is,
		maxConcurrentHash:     5,
		maxConcurrentContains: 64,
		maxConcurrentUpload:   8,
		containsBatchingDelay: 100 * time.Millisecond,
		containsBatchSize:     50,
		stage1DedupeChan:      make(chan *archiverItem, 1024),
		stage2HashChan:        make(chan *archiverItem, 10240),
		stage3LookupChan:      make(chan *archiverItem, 2048),
		stage4UploadChan:      make(chan *archiverItem, 2048),
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.stage1DedupeLoop()
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.stage2HashLoop()
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.stage3LookupLoop()
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.stage4UploadLoop()
	}()
	return a
}

// Private details.

// archiverItem is an item to process. Implements Future.
//
// It is caried over from pipeline stage to stage to do processing on it.
type archiverItem struct {
	// Immutable.
	path        string         // Set when source is a file on disk
	displayName string         // Name to use to qualify this item
	wgHashed    sync.WaitGroup // Released once .digestItem.Digest is set

	// Mutable.
	lock       sync.Mutex
	err        error               // Item specific error
	digestItem isolated.DigestItem // Mutated by hashLoop(), used by doContains()
	linked     []*archiverItem     // Deduplicated item.

	// Mutable but not accessible externally.
	src   io.ReadSeeker             // Source of data
	state *isolatedclient.PushState // Server-side push state for cache miss
}

func newArchiverItem(path, displayName string, src io.ReadSeeker) *archiverItem {
	i := &archiverItem{path: path, displayName: displayName, src: src}
	i.wgHashed.Add(1)
	return i
}

func (i *archiverItem) WaitForHashed() {
	i.wgHashed.Wait()
}

func (i *archiverItem) Error() error {
	i.lock.Lock()
	defer i.lock.Unlock()
	return i.err
}

func (i *archiverItem) Digest() isolated.HexDigest {
	i.lock.Lock()
	defer i.lock.Unlock()
	return i.digestItem.Digest
}

func (i *archiverItem) setErr(err error) {
	if err == nil {
		panic("internal error")
	}
	i.lock.Lock()
	defer i.lock.Unlock()
	if i.err == nil {
		i.err = err
		for _, child := range i.linked {
			child.lock.Lock()
			child.err = err
			child.lock.Unlock()
		}
	}
	// TODO(maruel): Support Close().
	i.src = nil
}

func (i *archiverItem) calcDigest() error {
	defer i.wgHashed.Done()
	var d isolated.DigestItem
	if i.path != "" {
		// Open and hash the file.
		var err error
		if d, err = isolated.HashFile(i.path); err != nil {
			i.setErr(err)
			return fmt.Errorf("hash(%s) failed: %s\n", i.displayName, err)
		}
	} else {
		// Use src instead.
		h := isolated.GetHash()
		size, err := io.Copy(h, i.src)
		if err != nil {
			i.setErr(err)
			return fmt.Errorf("read(%s) failed: %s\n", i.displayName, err)
		}
		_, err = i.src.Seek(0, os.SEEK_SET)
		if err != nil {
			i.setErr(err)
			return fmt.Errorf("seek(%s) failed: %s\n", i.displayName, err)
		}
		digest := isolated.HexDigest(hex.EncodeToString(h.Sum(nil)))
		d = isolated.DigestItem{digest, true, size}
	}
	i.lock.Lock()
	defer i.lock.Unlock()
	i.digestItem = d

	for _, child := range i.linked {
		child.lock.Lock()
		child.digestItem = d
		child.lock.Unlock()
		child.wgHashed.Done()
	}
	return nil
}

func (i *archiverItem) link(child *archiverItem) {
	child.lock.Lock()
	defer child.lock.Unlock()
	if child.src != nil || child.state != nil || child.linked != nil || child.err != nil {
		panic("internal error")
	}
	i.lock.Lock()
	defer i.lock.Unlock()
	i.linked = append(i.linked, child)
	if i.digestItem.Digest != "" {
		child.digestItem = i.digestItem
		child.err = i.err
		child.wgHashed.Done()
	} else if i.err != nil {
		child.err = i.err
		child.wgHashed.Done()
	}
}

// archiver archives content to an Isolate server.
//
// Uses a 4 stages pipeline, each doing work concurrently:
// - Deduplicating similar requests or known server hot cache hits.
// - Hashing files.
// - Batched cache hit lookups on the server.
// - Uploading cache misses.
type archiver struct {
	// Immutable.
	is                    isolatedclient.IsolateServer
	maxConcurrentHash     int           // Stage 2; Disk I/O bound.
	maxConcurrentContains int           // Stage 3; Server overload due to parallelism (DDoS).
	maxConcurrentUpload   int           // Stage 4; Network I/O bound.
	containsBatchingDelay time.Duration // Used by stage 3
	containsBatchSize     int           // Used by stage 3
	stage1DedupeChan      chan *archiverItem
	stage2HashChan        chan *archiverItem
	stage3LookupChan      chan *archiverItem
	stage4UploadChan      chan *archiverItem
	wg                    sync.WaitGroup
	canceler              common.Canceler

	// Mutable.
	lock  sync.Mutex
	stats Stats
}

// Close waits for all pending files to be done.
func (a *archiver) Close() error {
	// This is done so asynchronously calling push() won't crash.
	a.lock.Lock()
	close(a.stage1DedupeChan)
	a.stage1DedupeChan = nil
	a.lock.Unlock()

	a.wg.Wait()
	_ = a.canceler.Close()
	return a.CancelationReason()
}

func (a *archiver) Cancel(reason error) {
	a.canceler.Cancel(reason)
}

func (a *archiver) CancelationReason() error {
	return a.canceler.CancelationReason()
}

func (a *archiver) Push(displayName string, src io.ReadSeeker) Future {
	i := newArchiverItem("", displayName, src)
	if pos, err := i.src.Seek(0, os.SEEK_SET); pos != 0 || err != nil {
		i.err = errors.New("must use buffer set at offset 0")
		return i
	}
	return a.push(i)
}

func (a *archiver) PushFile(path string) Future {
	return a.push(newArchiverItem(path, path, nil))
}

func (a *archiver) Stats() *Stats {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.stats.deepCopy()
}

func (a *archiver) push(item *archiverItem) Future {
	a.lock.Lock()
	defer a.lock.Unlock()
	if a.stage1DedupeChan == nil {
		// Archiver was closed.
		return nil
	}
	a.stage1DedupeChan <- item
	return item
}

func (a *archiver) stage1DedupeLoop() {
	defer close(a.stage2HashChan)
	a.lock.Lock()
	stage1DedupeChan := a.stage1DedupeChan
	a.lock.Unlock()
	if stage1DedupeChan == nil {
		// This means Close() was called really too quickly.
		return
	}
	seen := map[string]*archiverItem{}
	for file := range stage1DedupeChan {
		item := file
		// TODO(todd): Create on-disk cache. This should be a separate interface
		// with its own implementation that 'archiver' keeps a reference to as
		// member.
		// TODO(maruel): Resolve symlinks for further deduplication? Depends on the
		// use case, not sure the trade off is worth.
		if item.path != "" {
			if previous, ok := seen[item.path]; ok {
				previous.link(item)
				continue
			}
		}
		a.stage2HashChan <- item
		seen[item.path] = item
	}
}

func (a *archiver) stage2HashLoop() {
	defer close(a.stage3LookupChan)
	pool := common.NewGoroutinePool(a.maxConcurrentContains, a.canceler)
	for file := range a.stage2HashChan {
		item := file
		pool.Schedule(func() {
			if err := item.calcDigest(); err != nil {
				a.Cancel(err)
				return
			}
			a.stage3LookupChan <- item
		})
	}

	_ = pool.Wait()
}

func (a *archiver) stage3LookupLoop() {
	defer close(a.stage4UploadChan)
	pool := common.NewGoroutinePool(a.maxConcurrentContains, a.canceler)
	items := []*archiverItem{}
	never := make(<-chan time.Time)
	timer := never
	loop := true
	for loop {
		select {
		case <-timer:
			batch := items
			pool.Schedule(func() {
				a.doContains(batch)
			})
			items = []*archiverItem{}
			timer = never

		case item, ok := <-a.stage3LookupChan:
			if !ok {
				loop = false
				break
			}
			items = append(items, item)
			if len(items) == a.containsBatchSize {
				batch := items
				pool.Schedule(func() {
					a.doContains(batch)
				})
				items = []*archiverItem{}
				timer = never
			} else if timer == never {
				timer = time.After(a.containsBatchingDelay)
			}
		}
	}

	if len(items) != 0 {
		batch := items
		pool.Schedule(func() {
			a.doContains(batch)
		})
	}
	_ = pool.Wait()
}

func (a *archiver) stage4UploadLoop() {
	pool := common.NewGoroutinePool(a.maxConcurrentUpload, a.canceler)
	for state := range a.stage4UploadChan {
		item := state
		pool.Schedule(func() {
			a.doUpload(item)
		})
	}
	_ = pool.Wait()
}

// doContains is called by stage 3.
func (a *archiver) doContains(items []*archiverItem) {
	tmp := make([]*isolated.DigestItem, len(items))
	// No need to lock each item at that point, no mutation occurs on
	// archiverItem.digestItem after stage 2.
	for i, item := range items {
		tmp[i] = &item.digestItem
	}
	states, err := a.is.Contains(tmp)
	if err != nil {
		err = fmt.Errorf("contains(%d) failed: %s", len(items), err)
		a.Cancel(err)
		for _, item := range items {
			item.setErr(err)
		}
		return
	}
	for index, state := range states {
		size := items[index].digestItem.Size
		if state == nil {
			a.lock.Lock()
			a.stats.Hits = append(a.stats.Hits, size)
			a.lock.Unlock()
		} else {
			a.lock.Lock()
			a.stats.Misses = append(a.stats.Misses, size)
			a.lock.Unlock()
			items[index].state = state
			a.stage4UploadChan <- items[index]
		}
	}
	log.Printf("Looked up %d items\n", len(items))
}

// doUpload is called by stage 4.
func (a *archiver) doUpload(item *archiverItem) {
	var src io.Reader
	if item.src == nil {
		f, err := os.Open(item.path)
		if err != nil {
			a.Cancel(err)
			item.setErr(err)
			return
		}
		defer f.Close()
		src = f
	} else {
		src = item.src
		item.src = nil
	}
	start := time.Now()
	if err := a.is.Push(item.state, src); err != nil {
		err = fmt.Errorf("push(%s) failed: %s\n", item.path, err)
		a.Cancel(err)
		item.setErr(err)
	}
	duration := time.Now().Sub(start)
	u := &UploadStat{duration, item.digestItem.Size}
	a.lock.Lock()
	a.stats.Pushed = append(a.stats.Pushed, u)
	a.lock.Unlock()
	log.Printf("Uploaded %.1fkb\n", float64(item.digestItem.Size)/1024.)
}
