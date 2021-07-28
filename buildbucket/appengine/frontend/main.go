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
	"strings"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/access"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"

	// Enable datastore transactional tasks support.
	_ "go.chromium.org/luci/server/tq/txn/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/rpc"
	pb "go.chromium.org/luci/buildbucket/proto"
)

// isBeefy returns whether the request was intended for the beefy service.
func isBeefy(req *http.Request) bool {
	return strings.Contains(req.Host, "beefy")
}

// isDev returns whether the request was intended for the dev instance.
func isDev(req *http.Request) bool {
	return strings.HasSuffix(req.Host, "-dev.appspot.com")
}

func main() {
	mods := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
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
		beefyURL, err := url.Parse(fmt.Sprintf("https://beefy-dot-%s.appspot.com/python", srv.Options.CloudProject))
		if err != nil {
			panic(err)
		}
		prx := httputil.NewSingleHostReverseProxy(pythonURL)
		prx.Director = func(req *http.Request) {
			target := pythonURL
			if isBeefy(req) {
				target = beefyURL
			}
			// According to net.Request documentation, setting Host is unnecessary
			// because URL.Host is supposed to be used for outbound requests.
			// However, on GAE, it seems that req.Host is incorrectly used.
			req.Host = target.Host
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = fmt.Sprintf("%s%s", target.Path, req.URL.Path)
		}
		// makeOverride returns a prpc.Override which allows the given percentage of requests
		// through to this service, proxying the remainder to Python.
		makeOverride := func(prodPct, devPct int) func(*router.Context) bool {
			return func(ctx *router.Context) bool {
				pct := prodPct
				if isDev(ctx.Request) {
					pct = devPct
				}
				switch val := ctx.Request.Header.Get("Should-Proxy"); val {
				case "true":
					pct = 0
					logging.Debugf(ctx.Context, "request demanded to be proxied")
				case "false":
					pct = 100
					logging.Debugf(ctx.Context, "request demanded not to be proxied")
				}
				if mathrand.Intn(ctx.Context, 100) < pct {
					return false
				}
				target := pythonURL
				if isBeefy(ctx.Request) {
					target = beefyURL
				}
				logging.Debugf(ctx.Context, "proxying request to %s", target)
				prx.ServeHTTP(ctx.Writer, ctx.Request)
				return true
			}
		}

		srv.PRPC.AccessControl = prpc.AllowOriginAll
		access.RegisterAccessServer(srv.PRPC, &access.UnimplementedAccessServer{})
		pb.RegisterBuildsServer(srv.PRPC, rpc.NewBuilds())
		pb.RegisterBuildersServer(srv.PRPC, rpc.NewBuilders())
		// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
		srv.PRPC.HackFixFieldMasksForJSON = true

		// Percentage of prod ScheduleBuild requests to serve from Go.
		pct := 10
		// Traffic split for unbatched ScheduleBuild requests. makeOverride(prod % -> Go, dev % -> Go).
		srv.PRPC.RegisterOverride("buildbucket.v2.Builds", "ScheduleBuild", makeOverride(pct, 100))
		// Traffic split for batched ScheduleBuild requests in prod (see rpc/batch.go).
		srv.Context = rpc.WithTrafficSplit(srv.Context, pct)

		cron.RegisterHandler("update_config", config.UpdateSettingsCfg)
		return nil
	})
}
