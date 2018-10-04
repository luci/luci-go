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
	"sync"
)

// Authenticator authenticates incoming pRPC requests.
type Authenticator interface {
	// Authenticate returns a context populated with authentication related
	// values.
	//
	// If the request is authenticated, 'Authenticate' returns a derived context
	// that gets passed to the RPC handler.
	//
	// If the request cannot be authenticated, 'Authenticate' returns nil context
	// and an error. A non-transient error triggers Unauthenticated grpc error,
	// a transient error triggers Internal grpc error. In both cases the error
	// message is set to err.Error().
	Authenticate(context.Context, *http.Request) (context.Context, error)
}

// nullAuthenticator implements Authenticator by doing nothing.
//
// See NoAuthentication variable.
type nullAuthenticator struct{}

func (nullAuthenticator) Authenticate(c context.Context, r *http.Request) (context.Context, error) {
	return c, nil
}

var defaultAuth = struct {
	sync.RWMutex
	Authenticator Authenticator
}{}

// RegisterDefaultAuth sets a default authenticator that is used unless
// Server.Authenticator is provided.
//
// It is supposed to be used during init() time, to configure the default
// authentication method used by the program.
//
// For example, see luci-go/appengine/gaeauth/server/prpc.go, that gets imported
// by LUCI GAE apps. It sets up authentication based on LUCI auth framework.
//
// Panics if 'a' is nil or a default authenticator is already set.
func RegisterDefaultAuth(a Authenticator) {
	if a == nil {
		panic("a is nil")
	}
	defaultAuth.Lock()
	defer defaultAuth.Unlock()
	if defaultAuth.Authenticator != nil {
		panic("default prpc authenticator is already set")
	}
	defaultAuth.Authenticator = a
}

// GetDefaultAuth returns the default authenticator set by RegisterDefaultAuth
// or nil if not registered.
func GetDefaultAuth() Authenticator {
	defaultAuth.RLock()
	defer defaultAuth.RUnlock()
	return defaultAuth.Authenticator
}
