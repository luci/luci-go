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
		maxConcurrentContains: 16,
		maxConcurrentUpload:   8,
		containsBatchingDelay: 100 * time.Millisecond,
		filesToHash:           make(chan *archiverItem, 10240),
		itemsToLookup:         make(chan *archiverItem, 10240),
		itemsToUpload:         make(chan *archiverItem, 10240),
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.hashLoop()
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.containsLoop()
	}()

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.uploadLoop()
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
	return nil
}

// archiver archives content to an Isolate server.
//
// Uses a 3 stages pipeline:
// - Hashing files concurrently.
// - Batched cache hit lookups on the server.
// - Uploading cache misses concurrently.
//
// TODO(maruel): Add another pipeline stage to do a local hash cache hit to
// skip hashing the files at all.
type archiver struct {
	// Immutable.
	is                    isolatedclient.IsolateServer
	maxConcurrentHash     int                // Stage 1
	maxConcurrentContains int                // Stage 2
	maxConcurrentUpload   int                // Stage 3
	containsBatchingDelay time.Duration      // Used by stage 2
	filesToHash           chan *archiverItem // Stage 1
	itemsToLookup         chan *archiverItem // Stage 2
	itemsToUpload         chan *archiverItem // Stage 3
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
	close(a.filesToHash)
	a.filesToHash = nil
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
	if a.filesToHash == nil {
		return nil
	}
	a.filesToHash <- item
	return item
}

// hashLoop is stage 1.
func (a *archiver) hashLoop() {
	defer close(a.itemsToLookup)

	pool := common.NewGoroutinePool(a.maxConcurrentHash, a.canceler)

	a.lock.Lock()
	filesToHash := a.filesToHash
	a.lock.Unlock()
	if filesToHash == nil {
		// This means Close() was called really too quickly.
		return
	}
	for file := range filesToHash {
		item := file
		pool.Schedule(func() {
			if err := item.calcDigest(); err != nil {
				a.Cancel(err)
				return
			}
			a.itemsToLookup <- item
		})
	}

	_ = pool.Wait()
}

// containsLoop is stage 2.
func (a *archiver) containsLoop() {
	defer close(a.itemsToUpload)

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

		case item, ok := <-a.itemsToLookup:
			if !ok {
				loop = false
				break
			}
			items = append(items, item)
			if timer == never {
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

// uploadLoop is stage 3.
func (a *archiver) uploadLoop() {
	pool := common.NewGoroutinePool(a.maxConcurrentUpload, a.canceler)

	for state := range a.itemsToUpload {
		item := state
		pool.Schedule(func() {
			a.doUpload(item)
		})
	}
	_ = pool.Wait()
}

// doContains is scaled by stage 2.
func (a *archiver) doContains(items []*archiverItem) {
	tmp := make([]*isolated.DigestItem, len(items))
	// No need to lock each item at that point, no mutation occurs on
	// archiverItem.digestItem after stage 1.
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
			a.itemsToUpload <- items[index]
		}
	}
	log.Printf("Looked up %d items\n", len(items))
}

// doUpload is scaled by stage 3.
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
