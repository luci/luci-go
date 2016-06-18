// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package archiver

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/client/internal/progress"
	"github.com/luci/luci-go/client/isolatedclient"
	"github.com/luci/luci-go/common/api/isolate/isolateservice/v1"
	"github.com/luci/luci-go/common/isolated"
	"github.com/luci/luci-go/common/tracer"
	"github.com/luci/luci-go/common/units"
)

const (
	groupFound      progress.Group   = 0
	groupFoundFound progress.Section = 0

	groupHash         progress.Group   = 1
	groupHashDone     progress.Section = 0
	groupHashDoneSize progress.Section = 1
	groupHashTodo     progress.Section = 2

	groupLookup     progress.Group   = 2
	groupLookupDone progress.Section = 0
	groupLookupTodo progress.Section = 1

	groupUpload         progress.Group   = 3
	groupUploadDone     progress.Section = 0
	groupUploadDoneSize progress.Section = 1
	groupUploadTodo     progress.Section = 2
	groupUploadTodoSize progress.Section = 3
)

var headers = [][]progress.Column{
	{{Name: "found"}},
	{
		{Name: "hashed"},
		{Name: "size", Formatter: units.SizeToString},
		{Name: "to hash"},
	},
	{{Name: "looked up"}, {Name: "to lookup"}},
	{
		{Name: "uploaded"},
		{Name: "size", Formatter: units.SizeToString},
		{Name: "to upload"},
		{Name: "size", Formatter: units.SizeToString},
	},
}

// Future is the future of a file pushed to be hashed.
type Future interface {
	// DisplayName is the name associated to the content.
	DisplayName() string
	// WaitForHashed hangs until the item hash is known.
	WaitForHashed()
	// Error returns any error that occured for this item if any.
	Error() error
	// Digest returns the calculated digest once calculated, empty otherwise.
	Digest() isolated.HexDigest
}

// Archiver is an high level interface to an isolatedclient.IsolateServer.
type Archiver interface {
	common.Canceler
	// Push schedules item upload to the isolate server.
	// Smaller priority value means earlier processing.
	Push(displayName string, src isolatedclient.Source, priority int64) Future
	// PushFile schedules file upload to the isolate server.
	// Smaller priority value means earlier processing.
	PushFile(displayName, path string, priority int64) Future
	Stats() *Stats
}

// UploadStat is the statistic for a single upload.
type UploadStat struct {
	Duration time.Duration
	Size     units.Size
	Name     string
}

// Stats is statistics from the Archiver.
type Stats struct {
	Hits   []units.Size  // Bytes; each item is immutable.
	Pushed []*UploadStat // Misses; each item is immutable.
}

// TotalHits is the number of cache hits on the server.
func (s *Stats) TotalHits() int {
	return len(s.Hits)
}

// TotalBytesHits is the number of bytes not uploaded due to cache hits on the
// server.
func (s *Stats) TotalBytesHits() units.Size {
	out := units.Size(0)
	for _, i := range s.Hits {
		out += i
	}
	return out
}

// TotalMisses returns the number of cache misses on the server.
func (s *Stats) TotalMisses() int {
	return len(s.Pushed)
}

// TotalBytesPushed returns the sum of bytes uploaded.
func (s *Stats) TotalBytesPushed() units.Size {
	out := units.Size(0)
	for _, i := range s.Pushed {
		out += i.Size
	}
	return out
}

func (s *Stats) deepCopy() *Stats {
	// Only need to copy the slice, not the items themselves.
	return &Stats{s.Hits, s.Pushed}
}

// New returns a thread-safe Archiver instance.
func New(is isolatedclient.IsolateServer, out io.Writer) Archiver {
	// TODO(maruel): Cache hashes and server cache presence.
	a := &archiver{
		canceler:              common.NewCanceler(),
		progress:              progress.New(headers, out),
		is:                    is,
		maxConcurrentHash:     5,
		maxConcurrentContains: 64,
		maxConcurrentUpload:   8,
		containsBatchingDelay: 100 * time.Millisecond,
		containsBatchSize:     50,
		stage1DedupeChan:      make(chan *archiverItem),
		stage2HashChan:        make(chan *archiverItem),
		stage3LookupChan:      make(chan *archiverItem),
		stage4UploadChan:      make(chan *archiverItem),
	}
	tracer.NewPID(a, "archiver")

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		a.stage1DedupeLoop()
	}()

	// TODO(todd): Create on-disk cache in a new stage inserted between stages 1
	// and 2. This should be a separate interface with its own implementation
	// that 'archiver' keeps a reference to as member.

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

	// Push an nil item to enforce stage1DedupeLoop() woke up. Otherwise this
	// could lead to a race condition if Close() is called too quickly.
	a.stage1DedupeChan <- nil
	return a
}

// Private details.

// archiverItem is an item to process. Implements Future.
//
// It is caried over from pipeline stage to stage to do processing on it.
type archiverItem struct {
	// Immutable.
	displayName string         // Name to use to qualify this item
	wgHashed    sync.WaitGroup // Released once .digestItem.Digest is set
	priority    int64          // Lower values - earlier hashing and uploading.
	path        string         // Set when the source is a file on disk.
	a           *archiver

	// Mutable.
	lock       sync.Mutex
	err        error                                    // Item specific error
	digestItem isolateservice.HandlersEndpointsV1Digest // Mutated by hashLoop(), used by doContains()
	linked     []*archiverItem                          // Deduplicated item.

	// Mutable but not accessible externally.
	source isolatedclient.Source     // Source of data
	state  *isolatedclient.PushState // Server-side push state for cache miss
}

func newArchiverItem(a *archiver, displayName, path string, source isolatedclient.Source, priority int64) *archiverItem {
	tracer.CounterAdd(a, "itemsProcessing", 1)
	i := &archiverItem{a: a, displayName: displayName, path: path, source: source, priority: priority}
	i.wgHashed.Add(1)
	return i
}

func (i *archiverItem) Close() error {
	tracer.CounterAdd(i.a, "itemsProcessing", -1)
	i.a = nil
	return nil
}

func (i *archiverItem) DisplayName() string {
	return i.displayName
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
	return isolated.HexDigest(i.digestItem.Digest)
}

func (i *archiverItem) isFile() bool {
	return len(i.path) != 0
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
}

func (i *archiverItem) calcDigest() error {
	defer i.wgHashed.Done()
	var d isolateservice.HandlersEndpointsV1Digest

	src, err := i.source()
	if err != nil {
		return fmt.Errorf("source(%s) failed: %s\n", i.DisplayName(), err)
	}
	defer src.Close()

	h := isolated.GetHash()
	size, err := io.Copy(h, src)
	if err != nil {
		i.setErr(err)
		return fmt.Errorf("read(%s) failed: %s\n", i.DisplayName(), err)
	}
	d = isolateservice.HandlersEndpointsV1Digest{Digest: string(isolated.Sum(h)), IsIsolated: true, Size: size}

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
	if !child.isFile() || child.state != nil || child.linked != nil || child.err != nil {
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
	closeLock             sync.Mutex
	stage1DedupeChan      chan *archiverItem
	stage2HashChan        chan *archiverItem
	stage3LookupChan      chan *archiverItem
	stage4UploadChan      chan *archiverItem
	wg                    sync.WaitGroup
	canceler              common.Canceler
	progress              progress.Progress

	// Mutable.
	statsLock sync.Mutex
	stats     Stats
}

// Close waits for all pending files to be done.
func (a *archiver) Close() error {
	// This is done so asynchronously calling push() won't crash.

	a.closeLock.Lock()
	ok := false
	if a.stage1DedupeChan != nil {
		close(a.stage1DedupeChan)
		a.stage1DedupeChan = nil
		ok = true
	}
	a.closeLock.Unlock()

	if !ok {
		return errors.New("was already closed")
	}
	a.wg.Wait()
	_ = a.progress.Close()
	_ = a.canceler.Close()
	err := a.CancelationReason()
	tracer.Instant(a, "done", tracer.Global, nil)
	return err
}

func (a *archiver) Cancel(reason error) {
	tracer.Instant(a, "cancel", tracer.Thread, tracer.Args{"reason": reason})
	a.canceler.Cancel(reason)
}

func (a *archiver) CancelationReason() error {
	return a.canceler.CancelationReason()
}

func (a *archiver) Channel() <-chan error {
	return a.canceler.Channel()
}

func (a *archiver) Push(displayName string, source isolatedclient.Source, priority int64) Future {
	return a.push(newArchiverItem(a, displayName, "", source, priority))
}

func (a *archiver) PushFile(displayName, path string, priority int64) Future {
	source := func() (io.ReadCloser, error) {
		return os.Open(path)
	}
	return a.push(newArchiverItem(a, displayName, path, source, priority))
}

func (a *archiver) Stats() *Stats {
	a.statsLock.Lock()
	defer a.statsLock.Unlock()
	return a.stats.deepCopy()
}

func (a *archiver) push(item *archiverItem) Future {
	if a.pushLocked(item) {
		tracer.Instant(a, "itemAdded", tracer.Thread, tracer.Args{"item": item.DisplayName()})
		tracer.CounterAdd(a, "itemsAdded", 1)
		a.progress.Update(groupFound, groupFoundFound, 1)
		return item
	}
	item.Close()
	return nil
}

func (a *archiver) pushLocked(item *archiverItem) bool {
	// The close(a.stage1DedupeChan) call is always occuring with the lock held.
	a.closeLock.Lock()
	defer a.closeLock.Unlock()
	if a.stage1DedupeChan == nil {
		// Archiver was closed.
		return false
	}
	// stage1DedupeLoop must never block and must be as fast as it can because it
	// is done while holding a.closeLock.
	a.stage1DedupeChan <- item
	return true
}

func (a *archiver) stage1DedupeLoop() {
	c := a.stage1DedupeChan
	defer close(a.stage2HashChan)
	seen := map[string]*archiverItem{}
	// Create our own goroutine-local channel buffer, which doesn't need to be
	// synchronized (unlike channels).
	buildUp := []*archiverItem{}
	for {
		// Pull or push an item, dependending if there is build up.
		var item *archiverItem
		ok := true
		if len(buildUp) == 0 {
			item, ok = <-c
		} else {
			select {
			case item, ok = <-c:
			case a.stage2HashChan <- buildUp[0]:
				// Pop first item from buildUp.
				buildUp = buildUp[1:]
				a.progress.Update(groupHash, groupHashTodo, 1)
			}
		}
		if !ok {
			break
		}
		if item == nil {
			continue
		}

		// This loop must never block and must be as fast as it can as it is
		// functionally equivalent to running with a.closeLock held.
		if err := a.CancelationReason(); err != nil {
			item.setErr(err)
			item.Close()
			item.wgHashed.Done()
			continue
		}
		// TODO(maruel): Resolve symlinks for further deduplication? Depends on the
		// use case, not sure the trade off is worth.
		if item.isFile() {
			if previous, ok := seen[item.path]; ok {
				previous.link(item)
				// TODO(maruel): Semantically weird.
				item.Close()
				continue
			}
		}

		buildUp = append(buildUp, item)
		seen[item.path] = item
	}

	// Take care of the build up after the channel closed.
	for _, item := range buildUp {
		if err := a.CancelationReason(); err != nil {
			item.setErr(err)
			item.wgHashed.Done()
			item.Close()
		} else {
			// The archiver is being closed, this has to happen synchronously.
			a.stage2HashChan <- item
			a.progress.Update(groupHash, groupHashTodo, 1)
		}
	}
}

func (a *archiver) stage2HashLoop() {
	defer close(a.stage3LookupChan)
	pool := common.NewGoroutinePriorityPool(a.maxConcurrentHash, a.canceler)
	defer func() {
		_ = pool.Wait()
	}()
	for file := range a.stage2HashChan {
		// This loop will implicitly buffer when stage1 is too fast by creating a
		// lot of hung goroutines in pool. This permits reducing the contention on
		// a.closeLock.
		// TODO(tandrii): Implement backpressure in GoroutinePool, e.g. when it
		// exceeds 20k or something similar.
		item := file
		pool.Schedule(item.priority, func() {
			// calcDigest calls setErr() and update wgHashed even on failure.
			end := tracer.Span(a, "hash", tracer.Args{"name": item.DisplayName()})
			if err := item.calcDigest(); err != nil {
				end(tracer.Args{"err": err})
				a.Cancel(err)
				item.Close()
				return
			}
			end(tracer.Args{"size": float64(item.digestItem.Size)})
			tracer.CounterAdd(a, "bytesHashed", float64(item.digestItem.Size))
			a.progress.Update(groupHash, groupHashDone, 1)
			a.progress.Update(groupHash, groupHashDoneSize, item.digestItem.Size)
			a.progress.Update(groupLookup, groupLookupTodo, 1)
			a.stage3LookupChan <- item
		}, func() {
			item.setErr(a.CancelationReason())
			item.wgHashed.Done()
			item.Close()
		})
	}
}

func (a *archiver) stage3LookupLoop() {
	defer close(a.stage4UploadChan)
	pool := common.NewGoroutinePool(a.maxConcurrentContains, a.canceler)
	defer func() {
		_ = pool.Wait()
	}()
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
			}, nil)
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
				}, nil)
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
		}, nil)
	}
}

func (a *archiver) stage4UploadLoop() {
	pool := common.NewGoroutinePriorityPool(a.maxConcurrentUpload, a.canceler)
	defer func() {
		_ = pool.Wait()
	}()
	for state := range a.stage4UploadChan {
		item := state
		pool.Schedule(item.priority, func() {
			a.doUpload(item)
		}, nil)
	}
}

// emptyBackgroundContext is a placeholder context for usage with the isolated
// client. A better implementation would use a real context to allow the isolate
// client calls to be Cancel'd. This could be done by wiring up
// archvier.canceller to a fake context which is passed to the isolated client,
// or it could be done by actually implementing real contexts in archiver (and
// deprecating the use of canceler).
var emptyBackgroundContext = context.Background()

// doContains is called by stage 3.
func (a *archiver) doContains(items []*archiverItem) {
	tmp := make([]*isolateservice.HandlersEndpointsV1Digest, len(items))
	// No need to lock each item at that point, no mutation occurs on
	// archiverItem.digestItem after stage 2.
	for i, item := range items {
		tmp[i] = &item.digestItem
	}
	states, err := a.is.Contains(emptyBackgroundContext, tmp)
	if err != nil {
		err = fmt.Errorf("contains(%d) failed: %s", len(items), err)
		a.Cancel(err)
		for _, item := range items {
			item.setErr(err)
		}
		return
	}
	a.progress.Update(groupLookup, groupLookupDone, int64(len(items)))
	for index, state := range states {
		size := items[index].digestItem.Size
		if state == nil {
			a.statsLock.Lock()
			a.stats.Hits = append(a.stats.Hits, units.Size(size))
			a.statsLock.Unlock()
			items[index].Close()
		} else {
			items[index].state = state
			a.progress.Update(groupUpload, groupUploadTodo, 1)
			a.progress.Update(groupUpload, groupUploadTodoSize, items[index].digestItem.Size)
			a.stage4UploadChan <- items[index]
		}
	}
	log.Printf("Looked up %d items\n", len(items))
}

// doUpload is called by stage 4.
func (a *archiver) doUpload(item *archiverItem) {
	start := time.Now()
	if err := a.is.Push(emptyBackgroundContext, item.state, item.source); err != nil {
		err = fmt.Errorf("push(%s) failed: %s\n", item.path, err)
		a.Cancel(err)
		item.setErr(err)
	} else {
		a.progress.Update(groupUpload, groupUploadDone, 1)
		a.progress.Update(groupUpload, groupUploadDoneSize, item.digestItem.Size)
	}
	item.Close()
	size := units.Size(item.digestItem.Size)
	u := &UploadStat{time.Since(start), size, item.DisplayName()}
	a.statsLock.Lock()
	a.stats.Pushed = append(a.stats.Pushed, u)
	a.statsLock.Unlock()
	log.Printf("Uploaded %7s: %s\n", size, item.DisplayName())
}
