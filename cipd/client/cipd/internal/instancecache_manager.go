// Copyright 2026 The LUCI Authors.
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

package internal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/client/cipd/reader"
	"go.chromium.org/luci/cipd/common"
)

// InstanceOpenMode describes how an InstanceRequest wants to open the instance.
type InstanceOpenMode byte

const (
	// Source (the default) indicates that the cache should just return the
	// pkg.Source without opening the file.
	Source InstanceOpenMode = iota

	// Instance indicates that the cache should open the pkg.Source as a
	// pkg.Instance.
	Instance

	// VerifiedInstance indicates that the cache should open the pkg.Source as a
	// pkg.Instance, but only after checking that it's hash matches the
	// InstanceID
	VerifiedInstance
)

// InstanceRequest is passed to RequestInstances.
type InstanceRequest struct {
	Context context.Context    // carries the cancellation signal
	Done    context.CancelFunc // called right before the result is enqueued
	Pin     common.Pin         // identifies the instance to fetch
	OpenAs  InstanceOpenMode   // indicates how the cache should open the instance file.
	State   any                // passed to the InstanceResult as is
}

// InstanceResult is a result of a completed InstanceRequest.
type InstanceResult struct {
	Context  context.Context // copied from the InstanceRequest
	Pin      common.Pin      // the original pin from the InstanceRequest.
	Err      error           // non-nil if failed to obtain the instance
	Source   pkg.Source      // set only if OpenAs was Source, must be closed by the caller
	Instance pkg.Instance    // set only if OpenAs was not Source, must be closed by the caller
	State    any             // copied from the InstanceRequest
}

// ManagedInstanceCache is a wrapper around an InstanceCache which allows
// background fetching.
//
// NOTE: The methods on ManagedInstanceCache are not thread safe, in the
// sense that you cannot call e.g. RequestInstances, WaitInstance, and/or Close
// concurrently. It is intended that one thread will call RequestInstances
// and/or WaitInstance multiple times in sequence - if the ManagedInstanceCache
// is configured with ParallelDownloads>0, then its own background goroutines
// will service the requests concurrently.
//
// It satisfies instance requests in an arbitrary order, possibly fetching
// multiple instances in parallel.
type ManagedInstanceCache struct {
	Cache *InstanceCache

	// ParallelDownloads limits how many parallel fetches can happen at once.
	//
	// The zero value means that no background goroutines will be used at all,
	// and [ManagedInstanceCache.WaitInstance] will directly process enqueued
	// InstanceRequests.
	ParallelDownloads int

	// ParallelDownloadsFastStart, if true, will cause the cache to immediately
	// start with `ParallelDownloads` fetches.
	//
	// If false, then the cache will 'ramp up' to `ParallelDownloads` number of
	// concurrent fetches. If you are using this InstanceCache with downstream
	// unpacking/installation of instances, then you want this to allow the
	// unpacking goroutines to start working as early as possible.
	ParallelDownloadsFastStart bool

	// fetchReq is !nil if we haven't launched yet.
	fetchReq     chan *InstanceRequest
	fetchPending atomic.Int32
	fetchRes     chan *InstanceResult // non-nil only if ParallelDownloads > 0
	fetchWG      sync.WaitGroup
}

// maybeLaunch prepares the cache for fetching packages in background.
//
// called implicitly by RequestInstances.
func (c *ManagedInstanceCache) maybeLaunch() {
	if c.ParallelDownloads < 0 {
		panic("ParallelDownloads must be non-negative")
	}

	if c.fetchReq != nil {
		return
	}

	// This is an effectively an unlimited buffer. At very least it should be
	// large enough to hold all requests submitted by RequestInstances(...) before
	// the first WaitInstance() call when ParallelDownloads == 0 and the channel
	// is drained only in WaitInstance(). When ParallelDownloads > 0 the buffer
	// size doesn't matter much.
	c.fetchReq = make(chan *InstanceRequest, 1000000)

	// If allowed to fetch in parallel, run the work pool that does the job.
	if c.ParallelDownloads > 0 {
		// The buffer size here controls how many packages are allowed to sit in the
		// "downloaded, but not yet installed" queue before the pipeline starts
		// blocking new downloads. Each such pending file takes some resources (at
		// least an open file descriptor and some memory), so it's better to limit
		// the buffer size by some reasonable number.
		c.fetchRes = make(chan *InstanceResult, 50)

		// Start with 1 allowed concurrent download. It will eventually ramp up to
		// ParallelDownloads with each finished download.
		//
		// This is a heuristic for `ensure`-type flows to make sure we download the
		// first package ASAP, so the installer has something to install while more
		// packages are being downloaded.
		//
		// When 'fast starting', just launch all workers immediately. This
		// accelerates flows such as cache preparation which do not have any
		// downstream process like unzipping and just want to fetch as quickly as
		// possible.
		//
		// NOTE: workersLeft can become up to c.ParallelDownloads negative, but
		// workers will only launch a new worker if they observe it as >= 0 after
		// decrementing it so we should only launch a max of c.ParallelDownloads
		// workers.
		var workersLeft atomic.Int32

		var worker func()
		worker = func() {
			for req := range c.fetchReq {
				c.fetchRes <- c.handleRequest(req)

				if workersLeft.Load() > 0 {
					if workersLeft.Add(-1) >= 0 {
						c.fetchWG.Go(worker)
					}
				}
			}
		}

		if c.ParallelDownloadsFastStart {
			for range c.ParallelDownloads {
				c.fetchWG.Go(worker)
			}
		} else {
			workersLeft.Store(int32(c.ParallelDownloads - 1))
			c.fetchWG.Go(worker)
		}
	}
}

// HasPendingFetches is true if there's any InstanceRequest which hasn't yet
// been retrieved via [ManagedInstanceCache.WaitInstance].
func (c *ManagedInstanceCache) HasPendingFetches() bool {
	return c.fetchPending.Load() > 0
}

// RequestInstances enqueues requests to fetch instances into the cache.
//
// The results can be obtained with [ManagedInstanceCache.WaitInstance].
// Each enqueued InstanceRequest will get an exactly one InstanceResult
// (perhaps with an error inside). The order of responses may not match the
// order of requests.
func (c *ManagedInstanceCache) RequestInstances(ctx context.Context, reqs []*InstanceRequest) {
	c.maybeLaunch()

	iids := make([]string, len(reqs))
	for i, req := range reqs {
		iids[i] = req.Pin.InstanceID
	}
	// Pre-mark any existing instances as used so they won't be GC'd by another
	// process before we actually get around to calling OpenAsSource for them.
	c.Cache.Touch(ctx, iids...)

	c.fetchPending.Add(int32(len(reqs)))
	for _, r := range reqs {
		c.fetchReq <- r
	}
}

// WaitInstance blocks until some InstanceRequest is available.
//
// If ParallelDownloads is 0, this will directly handle a single arbitrary
// request.
//
// Otherwise, this will block until *any* submitted request is completed by one
// of the background goroutines.
func (c *ManagedInstanceCache) WaitInstance() (res *InstanceResult) {
	defer c.fetchPending.Add(-1)

	if c.ParallelDownloads == 0 {
		// No parallelism allowed, fetch inline.
		return c.handleRequest(<-c.fetchReq)
	}

	// Otherwise wait for the work pool to finish a request.
	return <-c.fetchRes
}

// Close shuts down goroutines started, then closes the managed cache.
//
// The caller MUST ensure there are no pending fetches before making this call,
// or this will panic.
//
// Resets this InstanceCache to its initial state.
func (c *ManagedInstanceCache) Close(ctx context.Context) {
	if c.HasPendingFetches() {
		panic("closing a ManagedInstanceCache with some fetches still pending")
	}

	if c.fetchReq != nil {
		close(c.fetchReq)
		if c.ParallelDownloads > 0 {
			c.fetchWG.Wait()
		}
		c.fetchPending.Store(0)
		c.fetchReq = nil
		c.fetchRes = nil
	}

	c.Cache.Close(ctx)
}

// handleRequest handles an *InstanceRequest producing an *InstanceResult.
func (c *ManagedInstanceCache) handleRequest(req *InstanceRequest) *InstanceResult {
	if req.Done != nil {
		defer req.Done()
	}

	res := &InstanceResult{
		Context: req.Context,
		Pin:     req.Pin,
		Err:     req.Context.Err(),
		State:   req.State,
	}
	if res.Err != nil {
		return res
	}

	ctx := req.Context
	pin := req.Pin

	switch src, err := c.Cache.OpenAsSource(ctx, pin); {
	case err != nil:
		res.Err = err
	case req.OpenAs == Source:
		res.Source = src
	case req.OpenAs == Instance, req.OpenAs == VerifiedInstance:
		// Normally, the Fetcher will verify the hash when fetching from the
		// network. However, in the case where the on-disk state can't be trusted
		// (e.g. it is a prepared offline cache which doesn't have a trusted chain
		// of custody from preparation to usage), the caller may require that we
		// verify the hash when opening the instance.
		//
		// We don't do this by default because on slow systems (e.g. tiny ARM SoC,
		// like raspberry pi) the hash verification can take significant time.
		// Modern systems with nvme storage will see a couple GBps here.
		vm := reader.SkipHashVerification
		if req.OpenAs == VerifiedInstance {
			vm = reader.VerifyHash
		}
		instance, err := reader.OpenInstance(ctx, src, reader.OpenInstanceOpts{
			VerificationMode: vm,
			InstanceID:       pin.InstanceID,
		})

		if err != nil {
			// We join the src.Close error so the caller can detect
			// CorruptFileDeletionFailure, if it occurred.
			res.Err = errors.Join(err, src.Close(ctx, reader.IsCorruptionError(err)))
		} else {
			res.Instance = instance
		}
	default:
		panic(fmt.Sprintf("impossible: InstanceRequest.OpenAs: %x", req.OpenAs))
	}
	return res
}
