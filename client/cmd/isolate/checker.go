// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"log"
	"os"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/api/support/bundler"

	service "go.chromium.org/luci/common/api/isolate/isolateservice/v1"
	"go.chromium.org/luci/common/isolatedclient"
)

// isolateService is an internal interface to allow mocking of the
// isolatedclient.Client.
type isolateService interface {
	Contains(context.Context, []*service.HandlersEndpointsV1Digest) ([]*isolatedclient.PushState, error)
	Push(context.Context, *isolatedclient.PushState, isolatedclient.Source) error
}

// A Checker checks whether items are available on the server.
// It has a single implementation, *BundlingChecker. See BundlingChecker for method documentation.
type Checker interface {
	AddItem(item *Item, isolated bool, callback CheckerCallback)
	PresumeExists(item *Item)
	Close() error
}

// CheckerCallback is the callback used by Checker to indicate whether a file is
// present on the isolate server. If the item not present, the callback will be
// include the PushState necessary to upload it. Otherwise, the PushState will
// be nil.
type CheckerCallback func(*Item, *isolatedclient.PushState)

type checkerItem struct {
	item     *Item
	isolated bool
	callback CheckerCallback
}

// BundlingChecker uses the isolatedclient.Client to check whether items are available
// on the server.
// BundlingChecker methods are safe to call concurrently.
type BundlingChecker struct {
	ctx     context.Context
	svc     isolateService
	bundler *bundler.Bundler
	err     error

	existingDigests map[string]struct{} // set of digests of items that we presume to exist.

	Hit, Miss CountBytes
}

// CountBytes aggregates a count of files and the number of bytes in them.
type CountBytes struct {
	Count int
	Bytes int64
}

func (cb *CountBytes) addFile(size int64) {
	cb.Count++
	cb.Bytes += size
}

// NewChecker creates a new Checker with the given isolated client.
// The provided context is used to make all requests to the isolate server.
func NewChecker(ctx context.Context, client *isolatedclient.Client) *BundlingChecker {
	return newChecker(ctx, client)
}

func newChecker(ctx context.Context, svc isolateService) *BundlingChecker {
	c := &BundlingChecker{
		svc:             svc,
		ctx:             ctx,
		existingDigests: make(map[string]struct{}),
	}
	c.bundler = bundler.NewBundler(checkerItem{}, func(bundle interface{}) {
		items := bundle.([]checkerItem)
		if c.err != nil {
			for _, item := range items {
				// Drop any more incoming items.
				log.Printf("WARNING dropped %q from Checker", item.item.Path)
			}
			return
		}
		c.err = c.check(items)
	})
	c.bundler.DelayThreshold = 50 * time.Millisecond
	c.bundler.BundleCountThreshold = 50
	return c
}

// AddItem adds the given item to the checker for testing, and invokes the provided
// callback asynchronously. The isolated param indicates whether the given item
// represents a JSON isolated manifest (as opposed to a regular file).
// In the case of an error, the callback may never be invoked.
func (c *BundlingChecker) AddItem(item *Item, isolated bool, callback CheckerCallback) {
	if err := c.bundler.Add(checkerItem{item, isolated, callback}, 0); err != nil {
		// An error is only returned if the size is too big, but we always use
		// zero size so no error is possible.
		panic(err)
	}
}

// PresumeExists causes the Checker to report that item exists on the server.
func (c *BundlingChecker) PresumeExists(item *Item) {
	c.existingDigests[string(item.Digest)] = struct{}{}
}

func (c *BundlingChecker) exists(item *Item) bool {
	_, ok := c.existingDigests[string(item.Digest)]
	return ok
}

// Close shuts down the checker, blocking until all pending items have been
// checked with the server. Close returns the first error encountered during
// the checking process, if any.
// After Close has returned, Checker is guaranteed to no longer invoke any
// previously-provided callback.
func (c *BundlingChecker) Close() error {
	c.bundler.Flush()
	// After Close has returned, we know there are no outstanding running
	// checks.
	return c.err
}

// check is invoked from the bundler's handler. As such, it is only ever run
// one invocation at a time.
func (c *BundlingChecker) check(items []checkerItem) error {
	// We skip checking items on the server if we already know they exist.
	existingItems, toCheck := c.partitionByExistence(items)

	for i, pushState := range c.contains(toCheck) {
		c.handleCheckResult(toCheck[i], pushState)
	}

	for _, item := range existingItems {
		c.handleCheckResult(item, nil)
	}
	return nil
}

// partitionByExistence partitions items into two slices: one containing items that
// are known to already exist on the server, and the other containing the remaining items.
func (c *BundlingChecker) partitionByExistence(items []checkerItem) (existing, notExisting []checkerItem) {
	for _, item := range items {
		if c.exists(item.item) {
			existing = append(existing, item)
		} else {
			notExisting = append(notExisting, item)
		}
	}
	return existing, notExisting
}

// contains calls isolateService.Contains on the supplied checkerItems.
func (c *BundlingChecker) contains(items []checkerItem) []*isolatedclient.PushState {
	var digests []*service.HandlersEndpointsV1Digest
	for _, item := range items {
		digests = append(digests, &service.HandlersEndpointsV1Digest{
			Digest:     string(item.item.Digest),
			Size:       item.item.Size,
			IsIsolated: item.isolated,
		})
	}
	pushStates, err := c.svc.Contains(c.ctx, digests)
	if err != nil {
		// TODO(djd): propogate this more cleanly. At the moment, dropping
		// callbacks may cause the TAR archiver to hang.
		log.Printf("ERROR: isolate Contains call failed: %v", err)
		os.Exit(infraFailExit)
	}
	return pushStates
}

func (c *BundlingChecker) handleCheckResult(item checkerItem, pushState *isolatedclient.PushState) {
	// We recheck whether we have cached knowledge that the item exists,
	// since the cache may be more up to date than the server response.
	// e.g. if the first time an item is checked, it is checked multiple
	// times in the same Contains call, we may update the cache when
	// handling the response for the first item, before handling the
	// response for the second item.
	if c.exists(item.item) {
		pushState = nil
	}

	if pushState == nil {
		c.Hit.addFile(item.item.Size)
		// Don't bother asking the server about this item again.
		c.PresumeExists(item.item)
	} else {
		c.Miss.addFile(item.item.Size)
	}

	item.callback(item.item, pushState)
}
