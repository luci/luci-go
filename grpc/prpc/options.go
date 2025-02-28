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

package prpc

import (
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/retry"
)

// Options controls how RPC requests are sent.
type Options struct {
	// Retry controls a strategy for retrying transient RPC errors and deadlines.
	//
	// Each call to this factory should return a new retry.Iterator to use for
	// controlling retry loop of a single RPC call.
	Retry retry.Factory

	// UserAgent is the value of User-Agent HTTP header to use if the request
	// doesn't have "user-agent" outgoing metadata.
	//
	// If empty, DefaultUserAgent is used.
	UserAgent string

	// Insecure can be set to true to use HTTP instead of HTTPS.
	//
	// Useful for local tests.
	Insecure bool

	// PerRPCTimeout, if > 0, is a timeout that is applied to each call attempt.
	//
	// If the context passed to the call has a shorter deadline, this timeout will
	// not be applied. Otherwise, if this timeout is hit, the RPC call attempt
	// will be considered as failed transiently and it may be retried just like
	// any other transient error per Retry policy.
	PerRPCTimeout time.Duration

	// RequestFormat defines how to encode requests.
	//
	// The default value is FormatBinary (i.e. use wirepb protobuf encoding).
	//
	// Can be overridden on per-call basis via RequestFormat() call option.
	RequestFormat Format

	// ResponseFormat defines how the server should encode responses.
	//
	// The default value is FormatBinary (i.e. use wirepb protobuf encoding).
	//
	// Can be overridden on per-call basis via ResponseFormat() call option.
	ResponseFormat Format

	// Debug is a flag indicate if we want to print more logs for debug purpose.
	//
	// Right now we use this option to print the raw requests when certain
	// failures occur.
	Debug bool

	// These can be set only using *prpc.CallOption or some grpc.CallOption.

	resHeaderMetadata  *metadata.MD // destination for response HTTP headers.
	resTrailerMetadata *metadata.MD // destination for response HTTP trailers.
	expectedCodes      []codes.Code // list of non-OK grpc codes NOT to log

	// These are used internally.

	host        string     // a hostname of a service being called
	serviceName string     // a service being called
	methodName  string     // a method being called
	reqCodec    protoCodec // how to serialize the request
	respCodec   protoCodec // how to deserialize the response
}

// DefaultOptions are used if no options are specified in Client.
func DefaultOptions() *Options {
	return &Options{
		Retry: func() retry.Iterator {
			return &retry.ExponentialBackoff{
				Limited: retry.Limited{
					Delay:   time.Second,
					Retries: 5,
				},
			}
		},
	}
}

func (o *Options) apply(callOptions []grpc.CallOption) {
	for _, co := range callOptions {
		switch val := co.(type) {
		case grpc.HeaderCallOption:
			o.resHeaderMetadata = val.HeaderAddr
		case grpc.TrailerCallOption:
			o.resTrailerMetadata = val.TrailerAddr
		case grpc.StaticMethodCallOption:
			// Noop.
		case *CallOption:
			val.apply(o)
		default:
			panic(fmt.Sprintf("unsupported call option %T is used with pRPC client", co))
		}
	}
}

func (o *Options) resetResponseMetadata() {
	if o.resHeaderMetadata != nil {
		*o.resHeaderMetadata = nil
	}
	if o.resTrailerMetadata != nil {
		*o.resTrailerMetadata = nil
	}
}

// CallOption mutates Options.
type CallOption struct {
	grpc.CallOption
	// apply mutates options.
	apply func(*Options)
}

// ExpectedCode can be used to indicate that given non-OK codes may appear
// during normal successful call flow, and thus they must not be logged as
// erroneous.
//
// Only affects local logging, nothing else.
func ExpectedCode(codes ...codes.Code) *CallOption {
	return &CallOption{
		grpc.EmptyCallOption{},
		func(o *Options) {
			o.expectedCodes = append(o.expectedCodes, codes...)
		},
	}
}

// RequestFormat returns a CallOption that defines how to encode the request.
//
// This has no effect on the response encoding. Use ResponseFormat() call option
// to change it.
func RequestFormat(f Format) *CallOption {
	return &CallOption{
		grpc.EmptyCallOption{},
		func(o *Options) {
			o.RequestFormat = f
		},
	}
}

// ResponseFormat returns a CallOption that defines how the server should encode
// the response.
//
// This has no effect on the request encoding. Use RequestFormat() call option
// to change it.
func ResponseFormat(f Format) *CallOption {
	return &CallOption{
		grpc.EmptyCallOption{},
		func(o *Options) {
			o.ResponseFormat = f
		},
	}
}
