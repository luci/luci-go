// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package backend implements HTTP server that handles requests to 'backend'
// module.
//
// Handlers here are called only via GAE cron or task queues.
package backend

import (
	"net/http"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/router"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"

	"github.com/luci/luci-go/tokenserver/appengine/impl/delegation"
	"github.com/luci/luci-go/tokenserver/appengine/impl/machinetoken"
	"github.com/luci/luci-go/tokenserver/appengine/impl/services/admin/adminsrv"
	"github.com/luci/luci-go/tokenserver/appengine/impl/services/admin/certauthorities"
)

var (
	caServer    = certauthorities.NewServer()
	adminServer = adminsrv.NewServer()
)

func init() {
	r := router.New()
	basemw := gaemiddleware.BaseProd()

	gaemiddleware.InstallHandlers(r, basemw)

	r.GET("/internal/cron/read-config", basemw.Extend(gaemiddleware.RequireCron), readConfigCron)
	r.GET("/internal/cron/fetch-crl", basemw.Extend(gaemiddleware.RequireCron), fetchCRLCron)
	r.GET("/internal/cron/bqlog/machine-tokens-flush", basemw.Extend(gaemiddleware.RequireCron), flushMachineTokensLogCron)
	r.GET("/internal/cron/bqlog/delegation-tokens-flush", basemw.Extend(gaemiddleware.RequireCron), flushDelegationTokensLogCron)

	http.DefaultServeMux.Handle("/", r)
}

// readConfigCron is handler for /internal/cron/read-config GAE cron task.
func readConfigCron(c *router.Context) {
	// Don't override manually imported configs with 'nil' on devserver.
	if info.IsDevAppServer(c.Context) {
		c.Writer.WriteHeader(http.StatusOK)
		return
	}

	wg := sync.WaitGroup{}
	var errs [2]error

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, errs[0] = adminServer.ImportCAConfigs(c.Context, nil)
		if errs[0] != nil {
			logging.Errorf(c.Context, "ImportCAConfigs failed - %s", errs[0])
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, errs[1] = adminServer.ImportDelegationConfigs(c.Context, nil)
		if errs[1] != nil {
			logging.Errorf(c.Context, "ImportDelegationConfigs failed - %s", errs[1])
		}
	}()

	wg.Wait()

	// Retry cron job only on transient errors. On fatal errors let it rerun one
	// minute later, as usual, to avoid spamming logs with errors.
	c.Writer.WriteHeader(statusFromErrs(errs[:]))
}

// fetchCRLCron is handler for /internal/cron/fetch-crl GAE cron task.
func fetchCRLCron(c *router.Context) {
	list, err := caServer.ListCAs(c.Context, nil)
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
			_, err := caServer.FetchCRL(c.Context, &admin.FetchCRLRequest{Cn: cn})
			if err != nil {
				logging.Errorf(c.Context, "FetchCRL(%q) failed - %s", cn, err)
				errs[i] = err
			}
		}(i, cn)
	}
	wg.Wait()

	// Retry cron job only on transient errors. On fatal errors let it rerun one
	// minute later, as usual, to avoid spamming logs with errors.
	c.Writer.WriteHeader(statusFromErrs(errs))
}

// flushMachineTokensLogCron is handler for /internal/cron/bqlog/machine-tokens-flush.
func flushMachineTokensLogCron(c *router.Context) {
	// FlushTokenLog logs errors inside. We also do not retry on errors. It's fine
	// to wait and flush on the next iteration.
	machinetoken.FlushTokenLog(c.Context)
	c.Writer.WriteHeader(http.StatusOK)
}

// flushDelegationTokensLogCron is handler for /internal/cron/bqlog/delegation-tokens-flush.
func flushDelegationTokensLogCron(c *router.Context) {
	// FlushTokenLog logs errors inside. We also do not retry on errors. It's fine
	// to wait and flush on the next iteration.
	delegation.FlushTokenLog(c.Context)
	c.Writer.WriteHeader(http.StatusOK)
}

// statusFromErrs returns 500 if any of gRPC errors is codes.Internal.
func statusFromErrs(errs []error) int {
	for _, err := range errs {
		if grpc.Code(err) == codes.Internal {
			return http.StatusInternalServerError
		}
	}
	return http.StatusOK
}
