// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package frontend implements HTTP server that handles requests to default
// module.
//
// It stitches together all the code.
package frontend

import (
	"net/http"
	"sync"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"google.golang.org/appengine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/gaeconfig"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/discovery"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/prpc"

	"github.com/luci/luci-go/appengine/cmd/tokenserver/services/certauthorities"
	"github.com/luci/luci-go/appengine/cmd/tokenserver/services/serviceaccounts"
	"github.com/luci/luci-go/common/api/tokenserver/v1"
)

var (
	// caServer implements tokenserver.CertificateAuthorities RPC interface.
	caServer = &certauthorities.Server{
		ConfigFactory: func(c context.Context) (config.Interface, error) {
			// Use fake config data on dev server for simplicity.
			inf := info.Get(c)
			if inf.IsDevAppServer() {
				return memory.New(devServerConfigs(inf.AppID())), nil
			}
			return gaeconfig.New(c)
		},
	}

	// caServerWithAuth adds admin check to caServer.
	caServerWithAuth = &tokenserver.DecoratedCertificateAuthorities{
		Service: caServer,
		Prelude: adminPrelude("tokenserver.CertificateAuthorities"),
	}

	// serviceAccountsServer implements tokenserver.ServiceAccounts RPC interface.
	serviceAccountsServer = &serviceaccounts.Server{}

	// serviceAccountsServerWithAuth adds admin check to serviceAccountsServer.
	serviceAccountsServerWithAuth = &tokenserver.DecoratedServiceAccounts{
		Service: serviceAccountsServer,
		Prelude: adminPrelude("tokenserver.ServiceAccounts"),
	}
)

// adminPrelude returns a prelude that authorizes only administrators.
func adminPrelude(serviceName string) func(context.Context, string, proto.Message) (context.Context, error) {
	return func(c context.Context, method string, _ proto.Message) (context.Context, error) {
		logging.Infof(c, "%s: %q is calling %q", serviceName, auth.CurrentIdentity(c), method)
		switch admin, err := auth.IsMember(c, "administrators"); {
		case err != nil:
			return nil, grpc.Errorf(codes.Internal, "can't check ACL - %s", err)
		case !admin:
			return nil, grpc.Errorf(codes.PermissionDenied, "not an admin")
		}
		return c, nil
	}
}

func init() {
	router := httprouter.New()
	base := func(h middleware.Handler) httprouter.Handle {
		if !appengine.IsDevAppServer() {
			h = middleware.WithPanicCatcher(h)
		}
		return gaemiddleware.BaseProd(h)
	}

	// Install auth and config handlers.
	server.InstallHandlers(router, base)

	// The service has no UI, so just redirect to stock RPC explorer.
	router.GET("/", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		http.Redirect(w, r, "/rpcexplorer/", http.StatusFound)
	})

	// Optional warmup routes.
	router.GET("/_ah/warmup", base(warmupHandler))
	router.GET("/_ah/start", base(warmupHandler))

	// Backend routes used for cron and task queues.
	router.GET("/internal/cron/read-config", base(gaemiddleware.RequireCron(readConfigCron)))
	router.GET("/internal/cron/fetch-crl", base(gaemiddleware.RequireCron(fetchCRLCron)))

	// Install all RPC servers.
	var api prpc.Server
	tokenserver.RegisterCertificateAuthoritiesServer(&api, caServerWithAuth)
	tokenserver.RegisterServiceAccountsServer(&api, serviceAccountsServerWithAuth)
	discovery.Enable(&api)
	api.InstallHandlers(router, base)

	// Expose all this stuff.
	http.DefaultServeMux.Handle("/", router)
}

/// Routes.

// warmupHandler warms in-memory caches.
func warmupHandler(c context.Context, w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if err := server.Warmup(c); err != nil {
		panic(err) // let panic catcher deal with it
	}
	w.WriteHeader(http.StatusOK)
}

// readConfigCron is handler for /internal/cron/read-config GAE cron task.
func readConfigCron(c context.Context, w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if _, err := caServer.ImportConfig(c, nil); err != nil {
		panic(err) // let panic catcher deal with it
	}
	w.WriteHeader(http.StatusOK)
}

// fetchCRLCron is handler for /internal/cron/fetch-crl GAE cron task.
func fetchCRLCron(c context.Context, w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	list, err := caServer.ListCAs(c, nil)
	if err != nil {
		panic(err) // let panic catcher deal with it
	}

	// Fetch CRL of each active CA in parallel. In practice there are very few
	// CAs there (~= 1), so the risk of OOM is small.
	wg := sync.WaitGroup{}
	errs := make([]error, len(list.Cn))
	for i, cn := range list.Cn {
		wg.Add(1)
		go func(i int, cn string) {
			defer wg.Done()
			_, err := caServer.FetchCRL(c, &tokenserver.FetchCRLRequest{Cn: cn})
			if err != nil {
				logging.Errorf(c, "FetchCRL(%q) failed - %s", cn, err)
				errs[i] = err
			}
		}(i, cn)
	}
	wg.Wait()

	// Retry cron job only on transient errors. On fatal errors let it rerun one
	// minute later, as usual, to avoid spamming logs with errors.
	status := http.StatusOK
	for _, err = range errs {
		if grpc.Code(err) == codes.Internal {
			status = http.StatusInternalServerError
			break
		}
	}
	w.WriteHeader(status)
}
