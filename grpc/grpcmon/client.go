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

package grpcmon

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	gcode "google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
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

func NewInstrumentedProxy(client interface{}) grpc.ClientConnInterface {
	return &proxy{client}
}

type proxy struct {
	client interface{}
}

// Invoke performs a unary RPC and returns after the response is received
// into reply.
func (p *proxy) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	name := method[strings.LastIndex(method, "/")+1:]
	callable := reflect.ValueOf(p.client).MethodByName(name)
	callArgs := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(args),
		reflect.ValueOf(opts),
	}

	started := clock.Now(ctx)
	var err error
	defer func() {
		reportClientRPCMetrics(ctx, method, err, clock.Now(ctx).Sub(started))
	}()

	callRes := callable.CallSlice(callArgs)

	if len(callRes) != 2 {
		panic(fmt.Errorf("%d values in response, but 2 expected", len(callRes)))
	}
	if !callRes[1].IsNil() {
		err = callRes[1].Interface().(error)
		if err != nil {
			return err
		}
	}
	proto.Merge(reply.(proto.Message), callRes[0].Interface().(proto.Message))
	return nil
}

// NewStream begins a streaming RPC.
func (p *proxy) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, errors.New("NewStream is not supported")
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
