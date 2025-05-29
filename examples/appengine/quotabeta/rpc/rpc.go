// Copyright 2022 The LUCI Authors.
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

// Package rpc implements a rate-limited RPC service.
package rpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/runtime/protoiface"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	pb "go.chromium.org/luci/examples/appengine/quotabeta/proto"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/quotabeta"
)

// Demo implements pb.DemoServer. Requires a quotaconfig.Interface in the
// context for all method calls.
type Demo struct {
}

// Ensure Demo implements pb.DemoServer at compile-time.
var _ pb.DemoServer = &Demo{}

// GlobalRateLimit is globally limited to one request every 60 seconds. This
// quota can be reset at any time by calling GlobalQuotaReset. On success,
// returns an *emptypb.Empty, and on failure returns a codes.ResourceExhausted
// gRPC error.
func (*Demo) GlobalRateLimit(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	// global-rate-limit has a maximum of 60 resources. When below 60,
	// automatically replenishes one resource per second. By debiting
	// 60 resources on every call, enforce a rate limit of one request
	// every 60 seconds.
	updates := map[string]int64{
		"global-rate-limit": -60,
	}
	switch err := quota.UpdateQuota(ctx, updates, nil); err {
	case nil:
		return &emptypb.Empty{}, nil
	case quota.ErrInsufficientQuota:
		return nil, appstatus.Errorf(codes.ResourceExhausted, "global rate limit exceeded")
	default:
		return nil, errors.Fmt("quota.UpdateQuota: %w", err)
	}
}

// GlobalQuotaReset resets quota for calling GlobalRateLimit. Always succeeds,
// returning an *emptypb.Empty.
func (*Demo) GlobalQuotaReset(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	// global-rate-limit has a maximum of 60 resources. Credit all 60.
	updates := map[string]int64{
		"global-rate-limit": 60,
	}
	switch err := quota.UpdateQuota(ctx, updates, nil); err {
	case nil:
		return &emptypb.Empty{}, nil
	default:
		return nil, errors.Fmt("quota.UpdateQuota: %w", err)
	}
}

// PerUserRateLimit is limited to two requests every 60 seconds for a given user.
// This quota can be reset at any time by calling PerUserQuotaReset. On success,
// returns an *emptypb.Empty, and on failure returns a codes.ResourceExhausted
// gRPC error.
func (*Demo) PerUserRateLimit(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	// per-user-rate-limit/${user} has a maximum of 60 resources per user.
	// When below 60, automatically replenishes one resource per second. By
	// debiting 30 resources on every call, enforce a rate limit of two requests
	// every 60 seconds.
	updates := map[string]int64{
		"per-user-rate-limit/${user}": -30,
	}
	opts := &quota.Options{
		User: string(auth.CurrentIdentity(ctx)),
	}
	switch err := quota.UpdateQuota(ctx, updates, opts); err {
	case nil:
		return &emptypb.Empty{}, nil
	case quota.ErrInsufficientQuota:
		return nil, appstatus.Errorf(codes.ResourceExhausted, "per-user rate limit exceeded")
	default:
		return nil, errors.Fmt("quota.UpdateQuota: %w", err)
	}
}

// PerUserQuotaReset resets the caller's own quota for calling PerUserRateLimit.
// Always succeeds, returning an *emptypb.Empty.
func (*Demo) PerUserQuotaReset(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	// per-user-rate-limit has a maximum of 60 resources per user. Credit all 60.
	updates := map[string]int64{
		"per-user-rate-limit/${user}": 60,
	}
	opts := &quota.Options{
		User: string(auth.CurrentIdentity(ctx)),
	}
	switch err := quota.UpdateQuota(ctx, updates, opts); err {
	case nil:
		return &emptypb.Empty{}, nil
	default:
		return nil, errors.Fmt("quota.UpdateQuota: %w", err)
	}
}

// New returns a new pb.DemoServer.
func New() pb.DemoServer {
	return &pb.DecoratedDemo{
		// Prelude logs the details of every request.
		Prelude: func(ctx context.Context, methodName string, _ protoiface.MessageV1) (context.Context, error) {
			logging.Debugf(ctx, "%q called %q", auth.CurrentIdentity(ctx), methodName)
			return ctx, nil
		},

		Service: &Demo{},

		// Postlude logs non-GRPC errors, and returns them as gRPC internal errors.
		Postlude: func(ctx context.Context, _ string, _ protoiface.MessageV1, err error) error {
			return appstatus.GRPCifyAndLog(ctx, err)
		},
	}
}
