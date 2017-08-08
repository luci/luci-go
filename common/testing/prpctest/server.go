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

// Package prpctest is a package to facilitate pRPC testing by wrapping
// httptest with a pRPC Server.
package prpctest

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"

	"golang.org/x/net/context"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"
)

// Server is a pRPC test server.
type Server struct {
	prpc.Server

	// Base returns a middleware chain. It is handed the Context passed to
	// Start. If Base is nil, setContext will be used.
	Base func(context.Context) router.MiddlewareChain

	// HTTP is the active HTTP test server. It will be valid when the Server is
	// running.
	HTTP *httptest.Server
}

func setContext(c context.Context) router.MiddlewareChain {
	return router.NewMiddlewareChain(
		func(ctx *router.Context, next router.Handler) {
			ctx.Context = c
			next(ctx)
		},
	)
}

// Start starts the server. Any currently-registered services will be installed
// into the pRPC Server.
func (s *Server) Start(c context.Context) {
	// Clean up any active server.
	s.Close()

	s.Authenticator = prpc.NoAuthentication
	base := s.Base
	if base == nil {
		base = setContext
	}

	r := router.New()
	s.InstallHandlers(r, base(c))
	s.HTTP = httptest.NewServer(r)
}

// NewClient returns a prpc.Client configured to use the Server.
func (s *Server) NewClient() (*prpc.Client, error) {
	if s.HTTP == nil {
		return nil, errors.New("not running")
	}

	u, err := url.Parse(s.HTTP.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server URL: %s", err)
	}

	return &prpc.Client{
		Host: u.Host,
		Options: &prpc.Options{
			Insecure: true,
		},
	}, nil
}

// Close closes the Server, releasing any retained resources.
func (s *Server) Close() {
	if s.HTTP != nil {
		s.HTTP.Close()

		s.HTTP = nil
	}
}
