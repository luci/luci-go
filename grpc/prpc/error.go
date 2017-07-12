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
)

// protocolError is returned if a pRPC request is malformed.
type protocolError struct {
	err    error
	status int // HTTP status to use in response.
}

func (e *protocolError) Error() string {
	return fmt.Sprintf("pRPC: %s", e.err)
}

// withStatus wraps an error with an HTTP status.
// If err is nil, returns nil.
func withStatus(err error, status int) *protocolError {
	if _, ok := err.(*protocolError); ok {
		panic("protocolError in protocolError")
	}
	if err == nil {
		return nil
	}
	return &protocolError{err, status}
}

// errorf creates a new protocol error.
func errorf(status int, format string, a ...interface{}) *protocolError {
	return withStatus(fmt.Errorf(format, a...), status)
}
