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
	"context"
	"errors"
	"net"
	"net/http/httptest"
	"net/url"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/router"
)

// Server is a pRPC test server.
type Server struct {
	prpc.Server

	// HTTP is the active HTTP test server. It will be valid when the Server is
	// running.
	HTTP *httptest.Server

	// Host is the server address ("addr:port") if it is running.
	Host string
}

// Start starts the server using the given context as the root server context.
//
// Any currently-registered services will be installed into the pRPC Server.
func (s *Server) Start(ctx context.Context) {
	// Clean up any active server.
	s.Close()

	r := router.New()
	s.InstallHandlers(r, nil)
	s.HTTP = httptest.NewUnstartedServer(r)
	s.HTTP.Config.BaseContext = func(_ net.Listener) context.Context { return ctx }
	s.HTTP.Start()

	u, err := url.Parse(s.HTTP.URL)
	if err != nil {
		panic(err)
	}
	s.Host = u.Host
}

// NewClient returns a prpc.Client configured to use the Server.
func (s *Server) NewClient() (*prpc.Client, error) {
	return s.NewClientWithOptions(nil)
}

// NewClientWithOptions returns a prpc.Client configured to use the Server.
func (s *Server) NewClientWithOptions(opts *prpc.Options) (*prpc.Client, error) {
	if s.HTTP == nil {
		return nil, errors.New("not running")
	}

	if opts == nil {
		opts = &prpc.Options{Insecure: true}
	} else {
		cpy := *opts
		opts = &cpy
		opts.Insecure = true
	}

	return &prpc.Client{
		C:       s.HTTP.Client(),
		Host:    s.Host,
		Options: opts,
	}, nil
}

// Close closes the Server, releasing any retained resources.
func (s *Server) Close() {
	if s.HTTP != nil {
		s.HTTP.Close()
		s.HTTP = nil
	}
}
