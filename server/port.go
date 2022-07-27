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
	"fmt"
	"net"
	"net/http"
	"sync"

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
func (p *Port) nameForLog() string {
	var pfx string
	if p.listener != nil {
		pfx = "http://" + p.listener.Addr().String()
	} else {
		pfx = "-"
	}
	if p.opts.Name != "" {
		return fmt.Sprintf("%s [%s]", pfx, p.opts.Name)
	}
	return pfx
}

// httpServer lazy-initializes and returns http.Server for this port.
//
// Called from Server's Serve.
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
		return fallback // no need for an extra layer of per-host routing at all
	}

	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if router, ok := mapping[r.Host]; ok {
			router.ServeHTTP(rw, r)
		} else {
			fallback.ServeHTTP(rw, r)
		}
	})
}
