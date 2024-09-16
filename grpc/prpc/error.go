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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/grpc/prpc/prpcpb"
)

// ProtocolErrorDetails extracts pRPC protocol error details from a gRPC error
// if they are there.
//
// If err is not a gRPC error, or it is a gRPC but with no ErrorDetails
// attached, returns nil.
func ProtocolErrorDetails(err error) *prpcpb.ErrorDetails {
	if st, _ := status.FromError(err); st != nil {
		for _, any := range st.Details() {
			if details, ok := any.(*prpcpb.ErrorDetails); ok {
				return details
			}
		}
	}
	return nil
}

// protocolError is returned if a pRPC request or response are malformed.
//
// This error exists on a boundary between gRPC and HTTP protocols and thus has
// two codes: gRPC code (when it is interpreted by a gRPC client) and HTTP code
// (when it is interpreted by an HTTP client).
type protocolError struct {
	err    string     // the error message
	code   codes.Code // gRPC code to use in the response
	status int        // HTTP status to use in response
}

func (e *protocolError) Error() string {
	if strings.HasPrefix(e.err, "prpc:") {
		return e.err
	}
	return "prpc: " + e.err
}

// protocolErr creates a new protocol error for given gRPC status code.
//
// The error will be returned with the given HTTP status.
func protocolErr(code codes.Code, status int, format string, a ...any) *protocolError {
	if code == codes.OK {
		panic("need a real error code, not OK")
	}
	if status < 400 {
		panic("need an HTTP status code indicating an error")
	}
	return &protocolError{
		err:    fmt.Sprintf(format, a...),
		code:   code,
		status: status,
	}
}

// errResponseTooBig produces a gRPC error with "response is too big" error.
//
// It includes prpcpb.ErrorDetails attached to the status.
func errResponseTooBig(actual, limit int64) error {
	var st *status.Status
	if actual == 0 {
		st = status.Newf(codes.Unavailable, "prpc: the response size exceeds the client limit %d", limit)
	} else {
		st = status.Newf(codes.Unavailable, "prpc: the response size %d exceeds the client limit %d", actual, limit)
	}
	st, err := st.WithDetails(&prpcpb.ErrorDetails{
		Error: &prpcpb.ErrorDetails_ResponseTooBig{
			ResponseTooBig: &prpcpb.ResponseTooBig{
				ResponseSize:  actual,
				ResponseLimit: limit,
			},
		},
	})
	if err != nil {
		panic(fmt.Sprintf("unexpected error attaching details %s", err))
	}
	return st.Err()
}
