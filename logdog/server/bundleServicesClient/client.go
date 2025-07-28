// Copyright 2017 The LUCI Authors.
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

package bundleServicesClient

import (
	"context"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/api/support/bundler"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gae"
	"go.chromium.org/luci/common/logging"

	s "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
)

// The maximum, AppEngine request size, minus 1MB for overhead.
const maxBundleSize = gae.MaxRequestSize - (1024 * 1024) // 1MB

// Client is a LogDog Coordinator Services endpoint client that intercepts
// calls that can be batched and buffers them, sending them with the Batch
// RPC instead of their independent individual RPCs.
//
// The Context and CallOption set for the first intercepted call will be used
// when making the batch call; all other CallOption sets will be ignored.
//
// Bundling parameters can be controlled by modifying the Bundler prior to
// invoking it.
type Client struct {
	// ServicesClient is the Coordinator Services endpoint Client that is being
	// wrapped.
	s.ServicesClient

	// Starting from the time that the first message is added to a bundle, once
	// this delay has passed, handle the bundle.
	DelayThreshold time.Duration

	// Once a bundle has this many items, handle the bundle. Since only one
	// item at a time is added to a bundle, no bundle will exceed this
	// threshold, so it also serves as a limit.
	BundleCountThreshold int

	initBundlerOnce sync.Once
	bundler         *bundler.Bundler

	// outstanding is used to track outstanding RPCs. On Flush, the Client will
	// block pending completion of all outstanding RPCs.
	outstanding sync.WaitGroup
}

// RegisterStream implements ServicesClient.
func (c *Client) RegisterStream(ctx context.Context, in *s.RegisterStreamRequest, opts ...grpc.CallOption) (
	*s.RegisterStreamResponse, error) {
	resp, err := c.bundleRPC(ctx, opts, &s.BatchRequest_Entry{
		Value: &s.BatchRequest_Entry_RegisterStream{RegisterStream: in},
	})
	if err != nil {
		return nil, err
	}

	return resp.GetRegisterStream(), nil
}

// LoadStream implements ServicesClient.
func (c *Client) LoadStream(ctx context.Context, in *s.LoadStreamRequest, opts ...grpc.CallOption) (
	*s.LoadStreamResponse, error) {
	resp, err := c.bundleRPC(ctx, opts, &s.BatchRequest_Entry{
		Value: &s.BatchRequest_Entry_LoadStream{LoadStream: in},
	})
	if err != nil {
		return nil, err
	}

	return resp.GetLoadStream(), nil
}

// TerminateStream implements ServicesClient.
func (c *Client) TerminateStream(ctx context.Context, in *s.TerminateStreamRequest, opts ...grpc.CallOption) (
	*emptypb.Empty, error) {
	_, err := c.bundleRPC(ctx, opts, &s.BatchRequest_Entry{
		Value: &s.BatchRequest_Entry_TerminateStream{TerminateStream: in},
	})
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// ArchiveStream implements ServicesClient.
func (c *Client) ArchiveStream(ctx context.Context, in *s.ArchiveStreamRequest, opts ...grpc.CallOption) (
	*emptypb.Empty, error) {
	_, err := c.bundleRPC(ctx, opts, &s.BatchRequest_Entry{
		Value: &s.BatchRequest_Entry_ArchiveStream{ArchiveStream: in},
	})
	if err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// Flush flushes the Bundler. It should be called when terminating to ensure
// that buffered client requests have been completed.
func (c *Client) Flush() {
	c.initBundler()
	c.bundler.Flush()
	c.outstanding.Wait()
}

func (c *Client) initBundler() {
	c.initBundlerOnce.Do(func() {
		c.bundler = bundler.NewBundler(&batchEntry{}, c.bundlerHandler)

		c.bundler.DelayThreshold = c.DelayThreshold
		c.bundler.BundleCountThreshold = c.BundleCountThreshold
		c.bundler.BundleByteThreshold = maxBundleSize // Hard-coded.
	})
}

// bundleRPC adds req to the underlying Bundler, blocks until it completes, and
// returns its response.
func (c *Client) bundleRPC(ctx context.Context, opts []grpc.CallOption, req *s.BatchRequest_Entry) (*s.BatchResponse_Entry, error) {
	c.initBundler()

	be := &batchEntry{
		req:      req,
		ctx:      ctx,
		opts:     opts,
		complete: make(chan *s.BatchResponse_Entry, 1),
	}
	if err := c.addEntry(be); err != nil {
		return nil, err
	}

	resp := <-be.complete
	if e := resp.GetErr(); e != nil {
		return nil, e.ToError()
	}
	return resp, nil
}

func (c *Client) addEntry(be *batchEntry) error {
	return c.bundler.Add(be, proto.Size(be.req))
}

// bundleHandler is called when a bundle threshold has been met.
//
// This is a bundler.Bundler handler function. "iface" is []*batchEntry{}, a
// slice of the prototype passed into NewBundler.
//
// Note that "iface" is owned by this handler; the Bundler allocates a new
// slice after each bundle dispatch. Therefore, retention and mutation are safe.
func (c *Client) bundlerHandler(iface any) {
	entries := iface.([]*batchEntry)
	if len(entries) == 0 {
		return
	}

	ctx, opts := entries[0].ctx, entries[0].opts

	c.outstanding.Add(1)
	go func() {
		defer c.outstanding.Done()
		c.sendBundle(ctx, entries, opts...)
	}()
}

func (c *Client) sendBundle(ctx context.Context, entries []*batchEntry, opts ...grpc.CallOption) {
	req := s.BatchRequest{
		Req: make([]*s.BatchRequest_Entry, len(entries)),
	}
	for i, ent := range entries {
		req.Req[i] = ent.req
	}

	resp, err := c.ServicesClient.Batch(ctx, &req, opts...)

	// Supply a response to each blocking request. Note that "complete" is a
	// buffered channel, so this will not block.
	if err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to send RPC bundle.")

		// Error case: generate an error response from "err".
		for _, ent := range entries {
			e := s.MakeError(err)
			ent.complete <- &s.BatchResponse_Entry{
				Value: &s.BatchResponse_Entry_Err{Err: e},
			}
		}
		return
	}

	// We don't have a solution for a case where the Coordinator couldn't provide
	// a single response. We would infinitely continue retrying our initial
	// request set.
	//
	// This shouldn't happen, but if it does, make it visible.
	if len(resp.Resp) == 0 {
		panic(errors.New("batch response had zero entries"))
	}

	// Pair each response with its request.
	count := 0
	for _, r := range resp.Resp {
		// Handle error conditions.
		switch {
		case r.Index < 0, int(r.Index) >= len(entries):
			logging.Warningf(ctx, "Response included invalid index %d (%d entries).", r.Index, len(entries))
			continue

		case entries[r.Index] == nil:
			logging.Warningf(ctx, "Response included duplicate entry for index %d.", r.Index)
			continue
		}

		entries[r.Index].complete <- r
		entries[r.Index] = nil
		count++
	}

	// Fast path: if our count equals the number of entries, then we've processed
	// them all.
	if count == len(entries) {
		return
	}

	// Figure out which entries we didn't process and resubmit.
	count = 0
	for _, be := range entries {
		if be == nil {
			// Already processed.
			continue
		}

		if err := c.addEntry(be); err != nil {
			// This was already added successfully, so it can't fail here.
			panic(errors.Fmt("failed to re-add entry: %w", err))
		}
		count++
	}
	logging.Debugf(ctx, "Resubmitting %d unprocessed entr[y|ies].", count)
}

type batchEntry struct {
	req *s.BatchRequest_Entry

	ctx  context.Context
	opts []grpc.CallOption

	complete chan *s.BatchResponse_Entry
}
