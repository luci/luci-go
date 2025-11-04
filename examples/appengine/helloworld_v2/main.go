// Copyright 2019 The LUCI Authors.
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

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gomodule/redigo/redis"
	"go.opentelemetry.io/otel"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/examples/k8s/helloworld/apipb"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/analytics"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/encryptedcookies"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/loginsessions"
	"go.chromium.org/luci/server/mailer"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/templates"
	"go.chromium.org/luci/server/tq"

	// Use datastore as a backend for auth session and TQ transactions.
	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

var tracer = otel.Tracer("go.chromium.org/luci/example")

func main() {
	// Additional modules that extend the server functionality.
	modules := []module.Module{
		analytics.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		encryptedcookies.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		loginsessions.NewModuleFromFlags(),
		mailer.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		// When running locally, serve static files ourself.
		if !srv.Options.Prod {
			srv.Routes.Static("/static", nil, http.Dir("./static"))
		}

		// gRPC example.
		apipb.RegisterGreeterServer(srv, &greeterServer{})

		// Logging and tracing example.
		srv.Routes.GET("/log", nil, func(c *router.Context) {
			logging.Debugf(c.Request.Context(), "Hello debug world")

			ctx, span := tracer.Start(c.Request.Context(), "Testing")
			logging.Infof(ctx, "Hello info world")
			time.Sleep(100 * time.Millisecond)
			span.End()

			logging.Warningf(c.Request.Context(), "Hello warning world")
			c.Writer.Write([]byte("Hello, world"))

			logging.WithError(fmt.Errorf("boom")).Errorf(c.Request.Context(), "Hello error world")
		})

		// Redis example.
		//
		// To run Redis for tests locally (in particular on OSX):
		//   docker run --name redis -p 6379:6379 --restart always --detach redis
		//
		// Then launch the example with "... -redis-addr :6379".
		//
		// Note that it makes Redis port available on 0.0.0.0. This is a necessity
		// when using Docker-for-Mac. Don't put any sensitive stuff there (or make
		// sure your firewall is configured to block external connections).
		srv.Routes.GET("/redis", nil, func(c *router.Context) {
			conn, err := redisconn.Get(c.Request.Context())
			if err != nil {
				http.Error(c.Writer, err.Error(), 500)
				return
			}
			defer conn.Close()
			n, err := redis.Int(conn.Do("INCR", "testKey"))
			if err != nil {
				http.Error(c.Writer, err.Error(), 500)
				return
			}
			fmt.Fprintf(c.Writer, "%d", n)
		})

		// OpenID token checks (e.g. for PubSub authenticated push subscription).
		openIDCheck := auth.Authenticator{
			Methods: []auth.Method{
				&openid.GoogleIDTokenAuthMethod{
					AudienceCheck: openid.AudienceMatchesHost,
				},
			},
		}
		mw := router.NewMiddlewareChain(openIDCheck.GetMiddleware())
		srv.Routes.POST("/push", mw, func(c *router.Context) {
			logging.Infof(c.Request.Context(), "Authenticated as %s", auth.CurrentIdentity(c.Request.Context()))
			// TODO: check auth.CurrentIdentity(...) against an allowlist of allowed
			// callers, etc.
		})

		// Using ID tokens for authenticating outbound calls. This synthetic example
		// works on localhost only.
		srv.Routes.GET("/call", mw, func(c *router.Context) {
			tr, err := auth.GetRPCTransport(c.Request.Context(),
				auth.AsSelf,
				auth.WithIDTokenAudience("https://${host}"),
			)
			if err != nil {
				http.Error(c.Writer, err.Error(), 500)
				return
			}

			req, _ := http.NewRequest("POST", "http://127.0.0.1:8800/push", nil)
			req.Host = "example.com"

			resp, err := (&http.Client{Transport: tr}).Do(req)
			if err != nil {
				http.Error(c.Writer, err.Error(), 500)
				return
			}
			defer resp.Body.Close()
		})

		// An example of a site that uses encrypted cookies for authentication.
		templatesBundle := &templates.Bundle{
			Loader:    templates.FileSystemLoader(os.DirFS("templates")),
			DebugMode: func(context.Context) bool { return !srv.Options.Prod },
			DefaultArgs: func(ctx context.Context, e *templates.Extra) (templates.Args, error) {
				loginURL, err := auth.LoginURL(ctx, e.Request.URL.RequestURI())
				if err != nil {
					return nil, err
				}
				logoutURL, err := auth.LogoutURL(ctx, e.Request.URL.RequestURI())
				if err != nil {
					return nil, err
				}
				return templates.Args{
					"IsAnonymous":            auth.CurrentIdentity(ctx) == identity.AnonymousIdentity,
					"User":                   auth.CurrentUser(ctx),
					"LoginURL":               loginURL,
					"LogoutURL":              logoutURL,
					"GoogleAnalyticsSnippet": analytics.Snippet(ctx),
				}, nil
			},
		}
		htmlPageMW := router.NewMiddlewareChain(
			templates.WithTemplates(templatesBundle),
			auth.Authenticate(srv.CookieAuth),
		)

		srv.Routes.GET("/", htmlPageMW, func(c *router.Context) {
			templates.MustRender(c.Request.Context(), c.Writer, "pages/index.html", nil)
		})
		// To test redirects after login.
		srv.Routes.GET("/test/*something", htmlPageMW, func(c *router.Context) {
			templates.MustRender(c.Request.Context(), c.Writer, "pages/index.html", nil)
		})

		// Example of sending emails.
		srv.Routes.GET("/send-mail", nil, func(c *router.Context) {
			_, err := mailer.Send(c.Request.Context(), &mailer.Mail{
				To:       []string{"someone@example.com"},
				Subject:  "Hi",
				TextBody: "How are you doing?",
			})
			if err != nil {
				http.Error(c.Writer, err.Error(), 500)
			}
		})

		return nil
	})
}

type greeterServer struct{}

func (*greeterServer) SayHi(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	logging.Infof(ctx, "Hi")
	time.Sleep(100 * time.Millisecond)
	return &emptypb.Empty{}, nil
}
