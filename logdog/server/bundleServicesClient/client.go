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
	"sync"
	"time"

	"go.chromium.org/luci/common/errors"
	s "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/api/support/bundler"
	"google.golang.org/grpc"

	"golang.org/x/net/context"
)

// The maximum, AppEngine request size, extracted from:
// https://cloud.google.com/appengine/quotas#Requests
//
// We will remove 1MB for headers and overhead.
const maxBundleSize = 31 * 1024 * 1024 // 31MiB

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
	//
	// The default is bundler.DefaultDelayThreshold.
	DelayThreshold time.Duration

	// Once a bundle has this many items, handle the bundle. Since only one
	// item at a time is added to a bundle, no bundle will exceed this
	// threshold, so it also serves as a limit.
	//
	// The default is bundler.DefaultBundleCountThreshold.
	BundleCountThreshold int

	initBundlerOnce sync.Once
	bundler         *bundler.Bundler
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
	*empty.Empty, error) {

	_, err := c.bundleRPC(ctx, opts, &s.BatchRequest_Entry{
		Value: &s.BatchRequest_Entry_TerminateStream{TerminateStream: in},
	})
	if err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// ArchiveStream implements ServicesClient.
func (c *Client) ArchiveStream(ctx context.Context, in *s.ArchiveStreamRequest, opts ...grpc.CallOption) (
	*empty.Empty, error) {

	_, err := c.bundleRPC(ctx, opts, &s.BatchRequest_Entry{
		Value: &s.BatchRequest_Entry_ArchiveStream{ArchiveStream: in},
	})
	if err != nil {
		return nil, err
	}

	return &empty.Empty{}, nil
}

// Flush flushes the Bundler. It should be called when terminating to ensure
// that buffered client requests have been completed.
func (c *Client) Flush() {
	c.initBundler()
	c.bundler.Flush()
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
	if err := c.bundler.Add(be, proto.Size(be.req)); err != nil {
		return nil, err
	}

	resp := <-be.complete
	if e := resp.GetErr(); e != nil {
		return nil, e.ToError()
	}
	return resp, nil
}

func (c *Client) bundlerHandler(iface interface{}) {
	entries := iface.([]*batchEntry)
	if len(entries) == 0 {
		return
	}

	ctx, opts := entries[0].ctx, entries[0].opts
	req := s.BatchRequest{
		Req: make([]*s.BatchRequest_Entry, len(entries)),
	}
	for i, ent := range entries {
		req.Req[i] = ent.req
	}

	resp, err := c.ServicesClient.Batch(ctx, &req, opts...)
	if err == nil {
		// Determine other possible error values.
		switch {
		case resp == nil:
			err = errors.New("empty batch response")
		case len(resp.Resp) != len(entries):
			err = errors.Reason("response count (%d) doesn't match request count (%d)",
				len(resp.Resp), len(entries)).Err()
		}
	}

	// Supply a response to each blocking request. Note that "complete" is a
	// buffered channel, so this will not block.
	if err != nil {
		// Error case: generate an error response from "err".
		for _, ent := range entries {
			e := s.MakeError(err)
			ent.complete <- &s.BatchResponse_Entry{
				Value: &s.BatchResponse_Entry_Err{Err: e},
			}
		}
	} else {
		// Pair each response with its request.
		for i, ent := range entries {
			ent.complete <- resp.Resp[i]
		}
	}
}

type batchEntry struct {
	req *s.BatchRequest_Entry

	ctx  context.Context
	opts []grpc.CallOption

	complete chan *s.BatchResponse_Entry
}
