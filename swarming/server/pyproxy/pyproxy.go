// Copyright 2024 The LUCI Authors.
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

// Package pyproxy facilities migrating traffic from Python to Go.
package pyproxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/swarming/server/cfg"
)

// Proxy can proxy a portion of requests to Python.
type Proxy struct {
	cfg *cfg.Provider
	prx *httputil.ReverseProxy
}

// NewProxy sets up a proxy that send a portion of traffic to the Python host.
//
// Requests that hit the Go server are either handled by it or get proxied to
// the Python server, based on `traffic_migration` table in settings.cfg.
func NewProxy(cfg *cfg.Provider, pythonURL string) *Proxy {
	pyURL, err := url.Parse(pythonURL)
	if err != nil {
		panic(fmt.Sprintf("bad python URL: %s", err))
	}
	pyURL.Path = strings.TrimSuffix(pyURL.Path, "/")
	return &Proxy{
		cfg: cfg,
		prx: &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.Host = pyURL.Host
				req.URL.Scheme = pyURL.Scheme
				req.URL.Host = pyURL.Host
				req.URL.Path = pyURL.Path + req.URL.Path
			},
		},
	}
}

// DefaultOverride implements the default routing rule.
//
// It routes the request based on "X-Route-To" header (if present) or randomly
// with a configured probability (if the headers is absent).
//
// Returns true of the request was routed to Python and was already processed.
// Returns false if the request should be processed by Go.
func (m *Proxy) DefaultOverride(route string, rw http.ResponseWriter, req *http.Request) bool {
	ctx := req.Context()
	switch m.PickRouteBasedOnHeaders(ctx, req) {
	case "go":
		return false // i.e. don't override, let the Go handle it
	case "py":
		m.RouteToPython(rw, req)
		return true
	}
	return m.RandomizedOverride(ctx, route, rw, req)
}

// PickRouteBasedOnHeaders picks a backend based on the request headers.
//
// Returns either "go", "py" or "" (if headers indicate no preference). Logs
// the choice.
func (m *Proxy) PickRouteBasedOnHeaders(ctx context.Context, req *http.Request) string {
	// Just a precaution against malformed dispatch.yaml configuration. If the
	// request came from the Go proxy already somehow, do not proxy it again, i.e.
	// handle it in this Go server. Better than an infinite proxy loop.
	if req.Header.Get("X-Routed-From-Go") == "1" {
		logging.Errorf(ctx, "Proxy loop detected, refusing to proxy")
		return "go"
	}

	// Allow clients to ask for a specific backend. Useful in manual testing.
	if val := req.Header.Get("X-Route-To"); val == "py" || val == "go" {
		logging.Infof(ctx, "X-Route-To: %s => routing accordingly", val)
		return val
	}

	// If requests are hitting the Go hostname specifically (not the default GAE
	// app hostname routed via dispatch.yaml) let the Go server handle them.
	if strings.HasPrefix(req.Host, "default-go") {
		return "go"
	}

	// Decide based on the request body or randomly.
	return ""
}

// RandomizedOverride routes the request either to Python or Go based only on
// a random number generator (ignoring any headers).
//
// Returns true of the request was routed to Python and was already processed.
// Returns false if the request should be processed by Go.
func (m *Proxy) RandomizedOverride(ctx context.Context, route string, rw http.ResponseWriter, req *http.Request) bool {
	// Avoid hitting the rng when percent is 0% or 100% to make tests a little bit
	// less fragile. Only code paths that test really random routes will hit and
	// mutated the mocked rng. That way the rest of tests do not interfere with
	// tests that check randomness.
	percent := m.cfg.Cached(ctx).RouteToGoPercent(route)
	if percent != 0 && (percent == 100 || mathrand.Intn(ctx, 100) < percent) {
		// Do not log when 100% of requests are configured to be routed to Go, this
		// is the default end state. Just absence of this logging line is good
		// enough indicator that requests are handled by Go.
		if percent != 100 {
			logging.Infof(ctx, "Routing to Go (configured to route %d%% to Go)", percent)
		}
		return false
	}
	logging.Infof(ctx, "Routing to Python (configured to route %d%% to Go)", percent)
	m.RouteToPython(rw, req)
	return true
}

// RouteToPython sends the request to the Python server for handling.
func (m *Proxy) RouteToPython(rw http.ResponseWriter, req *http.Request) {
	req.Header.Set("X-Routed-From-Go", "1")
	m.prx.ServeHTTP(rw, req)
}
