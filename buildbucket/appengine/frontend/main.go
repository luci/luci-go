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

// Package main is the main entry point for the app.
package main

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/access"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/buildbucket/appengine/rpc"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func main() {
	mods := []module.Module{
		gaeemulation.NewModuleFromFlags(),
	}

	server.Main(nil, mods, func(srv *server.Server) error {
		// Proxy buildbucket.v2.Builds pRPC requests back to the Python
		// service in order to achieve a programmatic traffic split.
		// Because of the way dispatch routes work, requests are proxied
		// to a copy of the Python service hosted at a different path.
		// TODO(crbug/1042991): Remove the proxy once the go service handles all traffic.
		pythonURL, err := url.Parse(fmt.Sprintf("https://default-dot-%s.appspot.com/python", srv.Options.CloudProject))
		if err != nil {
			panic(err)
		}
		prx := httputil.NewSingleHostReverseProxy(pythonURL)
		prx.Director = func(req *http.Request) {
			// According to net.Request documentation, setting Host is unnecessary
			// because URL.Host is supposed to be used for outbound requests.
			// However, on GAE, it seems that req.Host is incorrectly used.
			req.Host = pythonURL.Host
			req.URL.Scheme = pythonURL.Scheme
			req.URL.Host = pythonURL.Host
			req.URL.Path = fmt.Sprintf("%s%s", pythonURL.Path, req.URL.Path)
		}

		access.RegisterAccessServer(srv.PRPC, &access.UnimplementedAccessServer{})
		buildbucketpb.RegisterBuildsServer(srv.PRPC, rpc.New())
		srv.PRPC.RegisterOverride("buildbucket.v2.Builds", "GetBuild", func(ctx *router.Context) bool {
			logging.Debugf(ctx.Context, "proxying request to %s", pythonURL)
			prx.ServeHTTP(ctx.Writer, ctx.Request)
			// TODO(crbug/1042991): Split some portion of traffic to the Go service.
			return false
		})
		return nil
	})
}
