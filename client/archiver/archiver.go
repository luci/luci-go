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
		filesToHash:           make(chan string, 10240),
		itemsToLookup:         make(chan *hashedItem, 10240),
		itemsToUpload:         make(chan *lookedUpItem, 10240),
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

type archiver struct {
	// Immutable.
	is                    isolatedclient.IsolateServer
	maxConcurrentHash     int
	maxConcurrentContains int
	maxConcurrentUpload   int
	containsBatchingDelay time.Duration
	filesToHash           chan string
	itemsToLookup         chan *hashedItem
	itemsToUpload         chan *lookedUpItem
	wg                    sync.WaitGroup

	// Mutable.
	lock  sync.Mutex
	err   error
	stats Stats
}

type hashedItem struct {
	digest isolated.DigestItem
	path   string
}

type lookedUpItem struct {
	state *isolatedclient.PushState
	path  string
	size  int64
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
	a.filesToHash <- path
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

func (a *archiver) hashLoop() {
	defer close(a.itemsToLookup)
	var wg sync.WaitGroup
	s := common.NewSemaphore(a.maxConcurrentHash)

	for file := range a.filesToHash {
		wg.Add(1)
		go func(f string) {
			if err := s.Wait(); err != nil {
				return
			}
			defer func() {
				s.Signal()
				wg.Done()
			}()
			d, err := isolated.HashFile(f)
			if err != nil {
				a.setErr(fmt.Errorf("hash(%s) failed: %s\n", f, err))
			} else {
				a.itemsToLookup <- &hashedItem{d, f}
			}
		}(file)
	}
	wg.Wait()
}

func (a *archiver) containsLoop() {
	defer close(a.itemsToUpload)
	var wg sync.WaitGroup
	s := common.NewSemaphore(a.maxConcurrentContains)

	items := []*hashedItem{}
	never := make(<-chan time.Time)
	timer := never
	loop := true
	for loop {
		select {
		case <-timer:
			wg.Add(1)
			go func(batch []*hashedItem) {
				if err := s.Wait(); err != nil {
					return
				}
				defer func() {
					s.Signal()
					wg.Done()
				}()
				a.doContains(batch)
			}(items)
			items = []*hashedItem{}
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
		go func(batch []*hashedItem) {
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

func (a *archiver) uploadLoop() {
	var wg sync.WaitGroup
	s := common.NewSemaphore(a.maxConcurrentUpload)

	for state := range a.itemsToUpload {
		wg.Add(1)
		go func(h *lookedUpItem) {
			if err := s.Wait(); err != nil {
				return
			}
			defer func() {
				s.Signal()
				wg.Done()
			}()
			a.doUpload(h)
		}(state)
	}
	wg.Wait()
}

func (a *archiver) doContains(items []*hashedItem) {
	tmp := make([]*isolated.DigestItem, len(items))
	for i, e := range items {
		tmp[i] = &e.digest
	}
	states, err := a.is.Contains(tmp)
	if err != nil {
		a.setErr(fmt.Errorf("contains(%d) failed: %s", len(items), err))
		return
	}
	for index, state := range states {
		size := items[index].digest.Size
		if state == nil {
			a.lock.Lock()
			a.stats.Hits = append(a.stats.Hits, size)
			a.lock.Unlock()
		} else {
			a.lock.Lock()
			a.stats.Misses = append(a.stats.Misses, size)
			a.lock.Unlock()
			a.itemsToUpload <- &lookedUpItem{state, items[index].path, size}
		}
	}
	log.Printf("Looked up %d items\n", len(items))
}

func (a *archiver) doUpload(l *lookedUpItem) {
	src, err := os.Open(l.path)
	if err != nil {
		a.setErr(fmt.Errorf("open(%s) failed: %s\n", l.path, err))
		return
	}
	defer src.Close()
	start := time.Now()
	if err = a.is.Push(l.state, src); err != nil {
		a.setErr(fmt.Errorf("push(%s) failed: %s\n", l.path, err))
		return
	}
	duration := time.Now().Sub(start)
	u := &UploadStat{duration, l.size}
	a.lock.Lock()
	a.stats.Pushed = append(a.stats.Pushed, u)
	a.lock.Unlock()
	log.Printf("Uploaded %.1fkb\n", float64(l.size)/1024.)
}
