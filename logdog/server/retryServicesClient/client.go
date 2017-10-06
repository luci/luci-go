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

package retryServicesClient

import (
	"time"

	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	s "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"golang.org/x/net/context"
)

// client wraps a services.ServicesClient, retrying transient errors.
type client struct {
	// Client is the CoordinatorClient that is being wrapped.
	c s.ServicesClient

	// f is the retry.Generator to use to generate retry.Iterator instances. If
	// nil, retry.Default will be used.
	f retry.Factory
}

// New wraps a supplied services.ServicesClient instance, automatically retrying
// transient errors.
//
// If the supplied retry factory is nil, retry.Default will be used.
func New(c s.ServicesClient, f retry.Factory) s.ServicesClient {
	if f == nil {
		f = retry.Default
	}
	return &client{c, transient.Only(f)}
}

func (c *client) GetConfig(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (r *s.GetConfigResponse, err error) {
	err = retry.Retry(ctx, c.f, func() (err error) {
		r, err = c.c.GetConfig(ctx, in, opts...)
		err = grpcutil.WrapIfTransient(err)
		return
	}, callback(ctx, "registering stream"))
	return
}

func (c *client) RegisterStream(ctx context.Context, in *s.RegisterStreamRequest, opts ...grpc.CallOption) (
	r *s.RegisterStreamResponse, err error) {

	err = retry.Retry(ctx, c.f, func() (err error) {
		r, err = c.c.RegisterStream(ctx, in, opts...)
		err = grpcutil.WrapIfTransient(err)
		return
	}, callback(ctx, "registering stream"))
	return
}

func (c *client) LoadStream(ctx context.Context, in *s.LoadStreamRequest, opts ...grpc.CallOption) (
	r *s.LoadStreamResponse, err error) {

	err = retry.Retry(ctx, c.f, func() (err error) {
		r, err = c.c.LoadStream(ctx, in, opts...)
		err = grpcutil.WrapIfTransient(err)
		return
	}, callback(ctx, "loading stream"))
	return
}

func (c *client) TerminateStream(ctx context.Context, in *s.TerminateStreamRequest, opts ...grpc.CallOption) (
	r *empty.Empty, err error) {
	err = retry.Retry(ctx, c.f, func() (err error) {
		r, err = c.c.TerminateStream(ctx, in, opts...)
		err = grpcutil.WrapIfTransient(err)
		return
	}, callback(ctx, "terminating stream"))
	return
}

func (c *client) ArchiveStream(ctx context.Context, in *s.ArchiveStreamRequest, opts ...grpc.CallOption) (
	r *empty.Empty, err error) {

	err = retry.Retry(ctx, c.f, func() (err error) {
		r, err = c.c.ArchiveStream(ctx, in, opts...)
		err = grpcutil.WrapIfTransient(err)
		return
	}, callback(ctx, "archiving stream"))
	return
}

func (c *client) Batch(ctx context.Context, in *s.BatchRequest, opts ...grpc.CallOption) (
	r *s.BatchResponse, err error) {

	err = retry.Retry(ctx, c.f, func() (err error) {
		r, err = c.c.Batch(ctx, in, opts...)
		err = grpcutil.WrapIfTransient(err)
		return
	}, callback(ctx, "sending batch"))
	return
}

func callback(ctx context.Context, op string) retry.Callback {
	return func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      d,
		}.Errorf(ctx, "Transient error %s. Retrying...", op)
	}
}
