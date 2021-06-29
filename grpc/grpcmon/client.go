// Copyright 2021 The LUCI Authors.
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

package grpcmon

import (
	"context"
	"time"

	gcode "google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	grpcClientCount = metric.NewCounter(
		"grpc/client/count",
		"Total number of RPCs.",
		nil,
		field.String("method"),         // full name of the grpc method
		field.String("canonical_code")) // grpc.Code of the result in string

	grpcClientDuration = metric.NewCumulativeDistribution(
		"grpc/client/duration",
		"Distribution of client-side RPC duration (in milliseconds).",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("method"),         // full name of the grpc method
		field.String("canonical_code")) // grpc.Code of the result in string

	rtKey = "Holds the current rpc tag"
)

// NewUnaryClientInterceptor returns an interceptor that gathers RPC call
// metrics and sends them to tsmon.
//
// It can be optionally chained with other interceptor. The reported metrics
// include time spent in this other interceptor too.
//
// Can be passed to a gRPC client via WithUnaryInterceptor(...) dial option.
//
// Use option.WithGRPCDialOption(grpc.WithUnaryInterceptor(...)) when
// instrumenting Google Cloud API clients.
//
// It assumes the RPC context has tsmon initialized already.
func NewUnaryClientInterceptor(next grpc.UnaryClientInterceptor) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
		started := clock.Now(ctx)
		defer func() {
			reportClientRPCMetrics(ctx, method, err, clock.Now(ctx).Sub(started))
		}()
		if next != nil {
			return next(ctx, method, req, reply, cc, invoker, opts...)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// reportClientRPCMetrics sends metrics after RPC call has finished.
func reportClientRPCMetrics(ctx context.Context, method string, err error, dur time.Duration) {
	code := status.Code(err)
	canon, ok := gcode.Code_name[int32(code)]
	if !ok {
		canon = code.String() // Code(%d)
	}
	grpcClientCount.Add(ctx, 1, method, canon)
	// TODO(tandrii): use dur.Milliseconds() once all GAE apps are >go1.11.
	grpcClientDuration.Add(ctx, dur.Seconds()*1e3, method, canon)
}

// ClientRPCStatsMonitor implements stats.Handler to update tsmon metrics with
// RPC stats.
//
// To chain this with other stats handler, use WithMultiStatsHandler.
type ClientRPCStatsMonitor struct{}

// TagRPC creates a context for the RPC.
//
// The context used for the rest lifetime of the RPC will be derived
// from the returned context.
//
// Can be passed to a gRPC client via WithStatsHandler(...) dial option.
func (m *ClientRPCStatsMonitor) TagRPC(ctx context.Context, tag *stats.RPCTagInfo) context.Context {
	return context.WithValue(ctx, &rtKey, tag)
}

// handleRPCEnd updates the metrics for an RPC completion.
func (m *ClientRPCStatsMonitor) handleRPCEnd(ctx context.Context, e *stats.End) {
	rt, ok := ctx.Value(&rtKey).(*stats.RPCTagInfo)
	if !ok {
		// This should never happen.
		panic("handleRPCEnd: missing rpc-tag")
	}
	reportClientRPCMetrics(ctx, rt.FullMethodName, e.Error, e.EndTime.Sub(e.BeginTime))
}

// HandleRPC processes the RPC stats.
func (m *ClientRPCStatsMonitor) HandleRPC(ctx context.Context, s stats.RPCStats) {
	switch event := s.(type) {
	case *stats.End:
		m.handleRPCEnd(ctx, event)
	default:
		// do nothing
		//
		// TODO(ddoman): handle InPayload and OutPayload to report
		// sent_msg, sent_msg_bytes, recv_msg, and recv_msg_bytes.
	}
}

// TagConn creates a context for the connection.
//
// The context passed to HandleConn will be derived from the returned context.
// The context passed to HandleRPC will NOT be derived from the returned context.
func (m *ClientRPCStatsMonitor) TagConn(ctx context.Context, t *stats.ConnTagInfo) context.Context {
	// do nothing
	return ctx
}

// HandleConn processes the Conn stats.
func (m *ClientRPCStatsMonitor) HandleConn(context.Context, stats.ConnStats) {
	// do nothing
}
