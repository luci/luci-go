// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package grpcmon

import (
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/metric"
	"github.com/luci/luci-go/common/tsmon/types"
)

var (
	grpcServerCount = metric.NewCounter(
		"grpc/server/count",
		"Total number of RPCs.",
		nil,
		field.String("method"), // full name of the grpc method
		field.Int("code"))      // grpc.Code of the result

	grpcServerDuration = metric.NewCumulativeDistribution(
		"grpc/server/duration",
		"Distribution of server-side RPC duration (in milliseconds).",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("method"), // full name of the grpc method
		field.Int("code"))      // grpc.Code of the result
)

// NewUnaryServerInterceptor returns an interceptor that gathers RPC handler
// metrics and sends them to tsmon.
//
// It can be optionally chained with other interceptor. The reported metrics
// include time spent in this other interceptor too.
//
// It assumes the RPC context has tsmon initialized already.
func NewUnaryServerInterceptor(next grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		started := clock.Now(ctx)
		panicing := true
		defer func() {
			// We don't want to recover anything, but we want to log Internal error
			// in case of a panic. We pray here reportServerRPCMetrics is very
			// lightweight and it doesn't panic itself.
			code := codes.OK
			switch {
			case err != nil:
				code = grpc.Code(err)
			case panicing:
				code = codes.Internal
			}
			reportServerRPCMetrics(ctx, info.FullMethod, code, clock.Now(ctx).Sub(started))
		}()
		if next != nil {
			resp, err = next(ctx, req, info, handler)
		} else {
			resp, err = handler(ctx, req)
		}
		panicing = false // normal exit, no panic happened, disarms defer
		return
	}
}

// reportServerRPCMetrics sends metrics after RPC handler has finished.
func reportServerRPCMetrics(ctx context.Context, method string, code codes.Code, dur time.Duration) {
	grpcServerCount.Add(ctx, 1, method, int(code))
	grpcServerDuration.Add(ctx, float64(dur.Nanoseconds()/1e6), method, int(code))
}
