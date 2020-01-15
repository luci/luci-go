// Copyright 2020 The LUCI Authors.
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

package usercontent

import (
	"context"
	"io"
	"net/http"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/middleware"
	"go.chromium.org/luci/server/router"
)

// Server can serve user content, and generate signed content URLs to the
// content.
type Server struct {
	// Included in generated signed URLs and required in content requests.
	Hostname string

	// used for isolate client
	anonClient, authClient *http.Client

	// mock for isolate fetching
	testFetchIsolate func(ctx context.Context, isolateHost, ns, digest string, w io.Writer) error
}

// NewServer creates a Server.
func NewServer(ctx context.Context, hostname string) (*Server, error) {
	selfTransport, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}

	return &Server{
		Hostname:   hostname,
		anonClient: http.DefaultClient,
		authClient: &http.Client{Transport: selfTransport},
	}, nil
}

// InstallHandlers installs handlers to serve user content.
func (s *Server) InstallHandlers(ctx context.Context, r *router.Router) {
	mc := router.NewMiddlewareChain(middleware.RequireHost(s.Hostname), validateToken)
	r.GET(isolatePathPattern, mc, s.handleIsolateContent)
}
