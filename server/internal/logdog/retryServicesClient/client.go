// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package retryServicesClient

import (
	"time"

	s "github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
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
	return &client{c, retry.TransientOnly(f)}
}

func (c *client) GetConfig(ctx context.Context, in *google.Empty, opts ...grpc.CallOption) (r *s.GetConfigResponse, err error) {
	err = retry.Retry(ctx, c.f, func() (err error) {
		r, err = c.c.GetConfig(ctx, in, opts...)
		return
	}, callback(ctx, "registering stream"))
	return
}

func (c *client) RegisterStream(ctx context.Context, in *s.RegisterStreamRequest, opts ...grpc.CallOption) (
	r *s.LogStreamState, err error) {
	err = retry.Retry(ctx, c.f, func() (err error) {
		r, err = c.c.RegisterStream(ctx, in, opts...)
		return
	}, callback(ctx, "registering stream"))
	return
}

func (c *client) TerminateStream(ctx context.Context, in *s.TerminateStreamRequest, opts ...grpc.CallOption) (
	r *google.Empty, err error) {
	err = retry.Retry(ctx, c.f, func() (err error) {
		r, err = c.c.TerminateStream(ctx, in, opts...)
		return
	}, callback(ctx, "terminating stream"))
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
