// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package prpctest is a package to facilitate pRPC testing by wrapping
// httptest with a pRPC Server.
package prpctest

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"

	"github.com/julienschmidt/httprouter"
	prpcCommon "github.com/luci/luci-go/common/prpc"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/prpc"
	"golang.org/x/net/context"
)

// Server is a pRPC test server.
type Server struct {
	prpc.Server

	// Base is the base middleware generator factory. It is handed the Context
	// passed to Start. If nil, middleware.TestingBase will be used.
	Base func(context.Context) middleware.Base

	// HTTP is the active HTTP test server. It will be valid when the Server is
	// running.
	HTTP *httptest.Server
}

// Start starts the server. Any currently-registered services will be installed
// into the pRPC Server.
func (s *Server) Start(c context.Context) {
	// Clean up any active server.
	s.Close()

	mwb := s.Base
	if mwb == nil {
		mwb = middleware.TestingBase
	}

	r := httprouter.New()
	s.InstallHandlers(r, mwb(c))
	s.HTTP = httptest.NewServer(r)
}

// NewClient returns a prpc.Client configured to use the Server.
func (s *Server) NewClient() (*prpcCommon.Client, error) {
	if s.HTTP == nil {
		return nil, errors.New("not running")
	}

	u, err := url.Parse(s.HTTP.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server URL: %s", err)
	}

	return &prpcCommon.Client{
		Host: u.Host,
		Options: &prpcCommon.Options{
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
