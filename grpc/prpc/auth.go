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
	"context"
	"net/http"
)

// Authenticator authenticates incoming pRPC requests.
type Authenticator interface {
	// AuthenticateHTTP returns a context populated with authentication related
	// values.
	//
	// If the request is authenticated, 'AuthenticateHTTP' returns a derived
	// context that gets passed to the RPC handler.
	//
	// If the request cannot be authenticated, 'AuthenticateHTTP' returns nil
	// context and an error. A non-transient error triggers Unauthenticated grpc
	// error, a transient error triggers Internal grpc error. In both cases the
	// error message is set to err.Error().
	AuthenticateHTTP(context.Context, *http.Request) (context.Context, error)
}

// nullAuthenticator implements Authenticator by doing nothing.
//
// See NoAuthentication variable.
type nullAuthenticator struct{}

func (nullAuthenticator) AuthenticateHTTP(c context.Context, r *http.Request) (context.Context, error) {
	return c, nil
}
