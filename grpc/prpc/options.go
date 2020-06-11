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
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/retry"
)

// Options controls how RPC requests are sent.
type Options struct {
	Retry retry.Factory // RPC retrial.

	// UserAgent is the value of User-Agent HTTP header.
	// If empty, DefaultUserAgent is used.
	UserAgent string
	Insecure  bool // if true, use HTTP instead of HTTPS.

	// PerRPCTimeout, if > 0, is a timeout that is applied to each RPC. If the
	// client Context has a shorter deadline, this timeout will not be applied.
	// Otherwise, if this timeout is hit, the RPC round will be considered
	// transient.
	PerRPCTimeout time.Duration

	// AcceptContentSubtype defines Content-Type over the wire for accepting
	// responses.
	// Valid values are "binary" and "json". Empty value defaults to "binary".
	// It can be overridden on per-call basis via CallAcceptContentSubtype().
	AcceptContentSubtype string

	// These can be set only using CallOption.

	resHeaderMetadata  *metadata.MD // destination for response HTTP headers.
	resTrailerMetadata *metadata.MD // destination for response HTTP trailers.
	expectedCodes      []codes.Code // list of non-OK grpc codes NOT to log

	// These are used internally.

	host        string // a hostname of a service being called
	serviceName string // a service being called
	methodName  string // a method being called
	inFormat    Format // encoding of the request
	outFormat   Format // encoding of the response
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
		prpcCo, ok := co.(*CallOption)
		if !ok {
			panic(fmt.Sprintf("non-pRPC call option %T is used with pRPC client", co))
		}
		prpcCo.apply(o)
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

// Header returns a CallOption that retrieves the header metadata.
// Can be used instead of with grpc.Header.
func Header(md *metadata.MD) *CallOption {
	return &CallOption{
		grpc.Header(md),
		func(o *Options) {
			o.resHeaderMetadata = md
		},
	}
}

// Trailer returns a CallOption that retrieves the trailer metadata.
// Can be used instead of grpc.Trailer.
func Trailer(md *metadata.MD) *CallOption {
	return &CallOption{
		grpc.Trailer(md),
		func(o *Options) {
			o.resTrailerMetadata = md
		},
	}
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

// CallAcceptContentSubtype returns a CallOption that sets Content-Type.
// For example, if content-subtype is "json", the Content-Type over the wire
// will be "application/json".
// Unlike that of the grpc.CallContentSubtype, sets Content-Type only for
// response, not for the request.
func CallAcceptContentSubtype(contentSubtype string) *CallOption {
	return &CallOption{
		grpc.EmptyCallOption{},
		func(o *Options) {
			o.AcceptContentSubtype = strings.ToLower(contentSubtype)
		},
	}
}
