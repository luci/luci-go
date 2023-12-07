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

package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/router"
)

// PortOptions is a configuration of a single serving HTTP port.
//
// See Server's AddPort.
type PortOptions struct {
	Name           string // optional logical name of the port for logs
	ListenAddr     string // local address to bind to or "-" for a dummy port
	DisableMetrics bool   // do not collect HTTP metrics for requests on this port
}

// Port is returned by Server's AddPort and used to setup the request routing.
//
// It represents an HTTP port with a request router.
type Port struct {
	// Routes is a router for requests hitting this port.
	//
	// This router is used for all requests whose Host header does not match any
	// specially registered per-host routers (see VirtualHost). Normally, there
	// are no such per-host routers, so usually Routes is used for all requests.
	//
	// Should be populated before Server's Serve loop.
	Routes *router.Router

	parent   *Server      // the owning server
	opts     PortOptions  // options passed to AddPort
	allowH2C bool         // if set allow HTTP/2 Cleartext requests
	listener net.Listener // listening socket if ListenAddr != "-"

	mu      sync.Mutex
	srv     *http.Server              // lazy-initialized in httpServer()
	perHost map[string]*router.Router // routers added in VirtualHost(...)
}

// VirtualHost returns a router (registering it if necessary) used for requests
// that have the given Host header.
//
// Note that requests that match some registered virtual host router won't
// reach the default router (port.Routes), even if the virtual host router
// doesn't have a route for them. Such requests finish with HTTP 404.
//
// Should be called before Server's Serve loop (panics otherwise).
func (p *Port) VirtualHost(host string) *router.Router {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.srv != nil {
		p.parent.Fatal(errors.Reason("the server has already been started").Err())
	}

	r := p.perHost[host]
	if r == nil {
		r = p.parent.newRouter(p.opts)
		if p.perHost == nil {
			p.perHost = make(map[string]*router.Router, 1)
		}
		p.perHost[host] = r
	}

	return r
}

// nameForLog returns a string to identify this port in the server logs.
//
// Part of the servingPort interface.
func (p *Port) nameForLog() string {
	var pfx string
	if p.listener != nil {
		pfx = "http://" + p.listener.Addr().String()
		if p.allowH2C {
			pfx += " (h2c on)"
		}
	} else {
		pfx = "-"
	}
	if p.opts.Name != "" {
		return fmt.Sprintf("%s [%s]", pfx, p.opts.Name)
	}
	return pfx
}

// serve runs the serving loop until it is gracefully stopped.
//
// Part of the servingPort interface.
func (p *Port) serve(baseCtx func() context.Context) error {
	srv := p.httpServer()
	srv.BaseContext = func(net.Listener) context.Context { return baseCtx() }
	err := srv.Serve(p.listener)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// shutdown gracefully stops the server, blocking until it is closed.
//
// Does nothing if the server is not running.
//
// Part of the servingPort interface.
func (p *Port) shutdown(ctx context.Context) {
	_ = p.httpServer().Shutdown(ctx)
}

// httpServer lazy-initializes and returns http.Server for this port.
//
// Once this function is called, no more virtual hosts can be added to the
// server (an attempt to do so causes a panic).
func (p *Port) httpServer() *http.Server {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.listener == nil {
		panic("httpServer must not be used with uninitialized ports")
	}

	if p.srv == nil {
		p.srv = &http.Server{
			Addr:     p.listener.Addr().String(),
			Handler:  p.initHandlerLocked(),
			ErrorLog: nil, // TODO(vadimsh): Log via 'logging' package.
		}
		// See https://pkg.go.dev/golang.org/x/net/http2/h2c.
		if p.allowH2C {
			p.srv.Handler = h2c.NewHandler(p.srv.Handler, &http2.Server{})
		}
	}
	return p.srv
}

// initHandlerLocked initializes the top-level router for the server.
func (p *Port) initHandlerLocked() http.Handler {
	// These are immutable once the server has started, so its fine to copy them
	// by pointer and use without any locking.
	mapping := p.perHost
	fallback := p.Routes

	if len(mapping) == 0 {
		// No need for an extra layer of per-host routing at all.
		return p.parent.wrapHTTPHandler(fallback)
	}

	return p.parent.wrapHTTPHandler(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if router, ok := mapping[r.Host]; ok {
			router.ServeHTTP(rw, r)
		} else {
			fallback.ServeHTTP(rw, r)
		}
	}))
}
