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

package rpcs

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	protov1 "github.com/golang/protobuf/proto"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/cursor"
)

// ConfigureMigration sets up proxy rules that send a portion of traffic to
// the Python host.
//
// Requests that hit the Go server are either handled by it or get proxied to
// the Python server, based on `traffic_migration` table in settings.cfg.
//
// Additionally all listing RPCs that use cursors are routed based on the cursor
// format. The Go server doesn't understand the Python cursor and vice-versa,
// so a listing started on e.g. Python server should be resumed there.
func ConfigureMigration(srv *prpc.Server, cfg *cfg.Provider, pythonURL string) {
	pyURL, err := url.Parse(pythonURL)
	if err != nil {
		panic(fmt.Sprintf("bad python URL: %s", err))
	}
	pyURL.Path = strings.TrimSuffix(pyURL.Path, "/")

	m := &migration{
		srv:        srv,
		cfg:        cfg,
		registered: stringset.New(0),
		prx: &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.Host = pyURL.Host
				req.URL.Scheme = pyURL.Scheme
				req.URL.Host = pyURL.Host
				req.URL.Path = pyURL.Path + req.URL.Path
			},
		},
	}

	// Bots RPCs.

	simpleRule(m, "swarming.v2.Bots", "GetBot")
	simpleRule(m, "swarming.v2.Bots", "DeleteBot")
	simpleRule(m, "swarming.v2.Bots", "TerminateBot")
	simpleRule(m, "swarming.v2.Bots", "CountBots")
	simpleRule(m, "swarming.v2.Bots", "GetBotDimensions")
	cursorCheckingRule[apipb.BotEventsRequest](m, "swarming.v2.Bots", "ListBotEvents")
	cursorCheckingRule[apipb.BotTasksRequest](m, "swarming.v2.Bots", "ListBotTasks")
	cursorCheckingRule[apipb.BotsRequest](m, "swarming.v2.Bots", "ListBots")

	// Tasks RPCs.

	simpleRule(m, "swarming.v2.Tasks", "GetResult")
	simpleRule(m, "swarming.v2.Tasks", "BatchGetResult")
	simpleRule(m, "swarming.v2.Tasks", "GetRequest")
	simpleRule(m, "swarming.v2.Tasks", "CancelTask")
	simpleRule(m, "swarming.v2.Tasks", "GetStdout")
	simpleRule(m, "swarming.v2.Tasks", "NewTask")
	simpleRule(m, "swarming.v2.Tasks", "ListTaskStates") // not really a listing
	simpleRule(m, "swarming.v2.Tasks", "CountTasks")
	cursorCheckingRule[apipb.TasksWithPerfRequest](m, "swarming.v2.Tasks", "ListTasks")
	cursorCheckingRule[apipb.TasksRequest](m, "swarming.v2.Tasks", "ListTaskRequests")
	cursorCheckingRule[apipb.TasksCancelRequest](m, "swarming.v2.Tasks", "CancelTasks")

	// Misc RPCs.

	simpleRule(m, "swarming.v2.Swarming", "GetDetails")
	simpleRule(m, "swarming.v2.Swarming", "GetToken")
	simpleRule(m, "swarming.v2.Swarming", "GetPermissions")

	// Buildbucket Task Backend RPCs.

	simpleRule(m, "buildbucket.v2.TaskBackend", "RunTask")
	simpleRule(m, "buildbucket.v2.TaskBackend", "FetchTasks")
	simpleRule(m, "buildbucket.v2.TaskBackend", "CancelTasks")
	simpleRule(m, "buildbucket.v2.TaskBackend", "ValidateConfigs")

	// Panic if some RPCs are not covered by the migration rules or there are
	// unrecognized rules.
	//
	// Note that buildbucket.v2.TaskBackend stub is generated using deprecated
	// gRPC protoc plugin and doesn't expose ServiceDesc, so we skip it here.
	m.assertVisited([]*grpc.ServiceDesc{
		&apipb.Bots_ServiceDesc,
		&apipb.Tasks_ServiceDesc,
		&apipb.Swarming_ServiceDesc,
	})
}

type migration struct {
	srv        *prpc.Server
	cfg        *cfg.Provider
	registered stringset.Set
	prx        *httputil.ReverseProxy
}

// simpleRule registers an override rule that doesn't look at the request body.
func simpleRule(m *migration, svc, method string) {
	route := fmt.Sprintf("/prpc/%s/%s", svc, method)

	if !m.registered.Add(route) {
		panic(fmt.Sprintf("already registered route %q", route))
	}

	m.srv.RegisterOverride(svc, method,
		func(rw http.ResponseWriter, req *http.Request, _ func(protov1.Message) error) (bool, error) {
			ctx := req.Context()
			switch m.pickRouteBasedOnHeaders(ctx, req) {
			case "go":
				return false, nil // i.e. don't override, let the Go handle it
			case "py":
				m.routeToPython(rw, req)
				return true, nil
			}
			return m.randomizedOverride(ctx, route, rw, req)
		},
	)
}

// cursorCheckingRule registers an override rule that deserializes the request
// and looks at the cursor to decide if the request should be routed to Python
// or Go.
func cursorCheckingRule[T any, TP interface {
	*T
	proto.Message
	GetCursor() string
}](m *migration, svc, method string) {
	route := fmt.Sprintf("/prpc/%s/%s", svc, method)

	if !m.registered.Add(route) {
		panic(fmt.Sprintf("already registered route %q", route))
	}

	m.srv.RegisterOverride(svc, method,
		func(rw http.ResponseWriter, req *http.Request, body func(msg protov1.Message) error) (bool, error) {
			ctx := req.Context()

			// Decide based on request headers before doing anything else.
			switch m.pickRouteBasedOnHeaders(ctx, req) {
			case "go":
				return false, nil // i.e. don't override, let the Go handle it
			case "py":
				m.routeToPython(rw, req)
				return true, nil
			}

			var msg T
			if err := body(protov1.MessageV1(&msg)); err != nil {
				// The body is weird. Route it using the default random rule and let
				// the real pRPC server handle the error, if any.
				logging.Warningf(ctx, "Failed to decode RPC body: %s", err)
			} else if cur := TP(&msg).GetCursor(); cur != "" {
				// Have a cursor, route to the same server that produced it.
				if yes, _ := cursor.IsValidCursor(ctx, cur); yes {
					logging.Infof(ctx, "Go cursor => routing to go")
					return false, nil // i.e. don't override, let the Go handle it
				}
				logging.Infof(ctx, "Python cursor => routing to py")
				m.routeToPython(rw, req)
				return true, nil
			}

			// If could not deserialize the request or the cursor is empty, pick the
			// backend randomly. This would initiate a chain of paginated requests
			// that will all hit the same implementation.
			return m.randomizedOverride(ctx, route, rw, req)
		},
	)
}

// pickRouteBasedOnHeaders picks a backend based on the request headers.
//
// Returns either "go", "py" or "" (if headers indicate no preference). Logs
// the choice.
func (m *migration) pickRouteBasedOnHeaders(ctx context.Context, req *http.Request) string {
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

// randomizedOverride routes the request either to Python or Go based only on
// a random number generator.
func (m *migration) randomizedOverride(ctx context.Context, route string, rw http.ResponseWriter, req *http.Request) (bool, error) {
	// Avoid hitting the rng when percent is 0% or 100% to make tests a little bit
	// less fragile. Only code paths that test really random routes will hit and
	// mutated the mocked rng. That way the rest of tests do not interfere with
	// tests that check randomness.
	percent := m.cfg.Cached(ctx).RouteToGoPercent(route)
	if percent != 0 && (percent == 100 || mathrand.Intn(ctx, 100) < percent) {
		logging.Infof(ctx, "Routing to Go (configured to route %d%% to Go)", percent)
		return false, nil
	}
	logging.Infof(ctx, "Routing to Python (configured to route %d%% to Go)", percent)
	m.routeToPython(rw, req)
	return true, nil
}

// routeToPython sends the request to the Python server for handling.
func (m *migration) routeToPython(rw http.ResponseWriter, req *http.Request) {
	req.Header.Set("X-Routed-From-Go", "1")
	m.prx.ServeHTTP(rw, req)
}

// assertVisited panics if some of the routes were not registered.
func (m *migration) assertVisited(svcs []*grpc.ServiceDesc) {
	expected := stringset.New(0)
	for _, svc := range svcs {
		for _, desc := range svc.Methods {
			expected.Add(fmt.Sprintf("/prpc/%s/%s", svc.ServiceName, desc.MethodName))
		}
	}
	expected.Difference(m.registered).Iter(func(route string) bool {
		panic(fmt.Sprintf("route %q is not registered in the rpcs/migration.go", route))
	})
}
