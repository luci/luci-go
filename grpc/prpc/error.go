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

	"google.golang.org/grpc/codes"
)

// protocolError is returned if a pRPC request is malformed.
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
	return fmt.Sprintf("pRPC: %s", e.err)
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
