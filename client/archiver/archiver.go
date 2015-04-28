// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package archiver

import (
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

// Archiver is an high level interface to an isolatedclient.IsolateServer.
type Archiver interface {
	io.Closer
	PushFile(path string)
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

	// Mutable.
	lock  sync.Mutex
	err   error
	stats Stats
}

// archiverItem is an item to process.
//
// It is caried over from pipeline stage to stage to do processing on it.
type archiverItem struct {
	// Immutable.
	path string

	// Mutable.
	lock       sync.Mutex
	err        error               // Item specific error
	digestItem isolated.DigestItem // Mutated by hashLoop(), used by doContains()

	// Mutable but not accessible externally.
	state *isolatedclient.PushState // Server-side push state for cache miss
}

func newArchiverItem(path string) *archiverItem {
	i := &archiverItem{path: path}
	return i
}

func (i *archiverItem) setDigest(d isolated.DigestItem) {
	i.lock.Lock()
	defer i.lock.Unlock()
	i.digestItem = d
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
}

// Close waits for all pending files to be done.
func (a *archiver) Close() error {
	close(a.filesToHash)
	a.wg.Wait()

	a.lock.Lock()
	defer a.lock.Unlock()
	return a.err
}

func (a *archiver) PushFile(path string) {
	item := newArchiverItem(path)
	a.filesToHash <- item
}

func (a *archiver) Stats() *Stats {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.stats.deepCopy()
}

func (a *archiver) setErr(err error) {
	if err == nil {
		panic("internal error")
	}
	log.Printf("%s\n", err)
	a.lock.Lock()
	defer a.lock.Unlock()
	if a.err == nil {
		a.err = err
	}
}

// hashLoop is stage 1.
func (a *archiver) hashLoop() {
	defer close(a.itemsToLookup)
	var wg sync.WaitGroup
	s := common.NewSemaphore(a.maxConcurrentHash)

	for file := range a.filesToHash {
		wg.Add(1)
		go func(item *archiverItem) {
			if err := s.Wait(); err != nil {
				return
			}
			defer func() {
				s.Signal()
				wg.Done()
			}()

			d, err := isolated.HashFile(item.path)
			if err != nil {
				item.setErr(err)
				a.setErr(fmt.Errorf("hash(%s) failed: %s\n", item.path, err))
				return
			}
			item.setDigest(d)
			a.itemsToLookup <- item
		}(file)
	}
	wg.Wait()
}

// containsLoop is stage 2.
func (a *archiver) containsLoop() {
	defer close(a.itemsToUpload)
	var wg sync.WaitGroup
	s := common.NewSemaphore(a.maxConcurrentContains)

	items := []*archiverItem{}
	never := make(<-chan time.Time)
	timer := never
	loop := true
	for loop {
		select {
		case <-timer:
			wg.Add(1)
			go func(batch []*archiverItem) {
				if err := s.Wait(); err != nil {
					return
				}
				defer func() {
					s.Signal()
					wg.Done()
				}()
				a.doContains(batch)
			}(items)
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
		wg.Add(1)
		go func(batch []*archiverItem) {
			if err := s.Wait(); err != nil {
				return
			}
			defer func() {
				s.Signal()
				wg.Done()
			}()
			a.doContains(batch)
		}(items)
	}
	wg.Wait()
}

// uploadLoop is stage 3.
func (a *archiver) uploadLoop() {
	var wg sync.WaitGroup
	s := common.NewSemaphore(a.maxConcurrentUpload)

	for item := range a.itemsToUpload {
		wg.Add(1)
		go func(item *archiverItem) {
			if err := s.Wait(); err != nil {
				return
			}
			defer func() {
				s.Signal()
				wg.Done()
			}()
			a.doUpload(item)
		}(item)
	}
	wg.Wait()
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
		a.setErr(err)
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
	src, err := os.Open(item.path)
	if err == nil {
		defer src.Close()
		start := time.Now()
		if err := a.is.Push(item.state, src); err != nil {
			err = fmt.Errorf("push(%s) failed: %s\n", item.path, err)
		} else {
			duration := time.Now().Sub(start)
			u := &UploadStat{duration, item.digestItem.Size}
			a.lock.Lock()
			a.stats.Pushed = append(a.stats.Pushed, u)
			a.lock.Unlock()
			log.Printf("Uploaded %.1fkb\n", float64(item.digestItem.Size)/1024.)
		}
	}
	if err != nil {
		a.setErr(err)
		item.setErr(err)
	}
}
