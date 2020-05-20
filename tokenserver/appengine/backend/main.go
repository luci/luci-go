// Copyright 2017 The LUCI Authors.
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

// Binary backend implements HTTP server that handles requests to 'backend'
// module.
//
// Handlers here are called only via GAE cron or task queues.
package main

import (
	"context"
	"net/http"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/appengine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/tokenserver/api/admin/v1"

	"go.chromium.org/luci/tokenserver/appengine/impl/delegation"
	"go.chromium.org/luci/tokenserver/appengine/impl/machinetoken"
	"go.chromium.org/luci/tokenserver/appengine/impl/projectscope"
	"go.chromium.org/luci/tokenserver/appengine/impl/serviceaccounts"
	"go.chromium.org/luci/tokenserver/appengine/impl/serviceaccountsv2"
	"go.chromium.org/luci/tokenserver/appengine/impl/services/admin/adminsrv"
	"go.chromium.org/luci/tokenserver/appengine/impl/services/admin/certauthorities"
)

var (
	caServer    = certauthorities.NewServer()
	adminServer = adminsrv.NewServer()
)

func main() {
	r := router.New()
	basemw := standard.Base()

	standard.InstallHandlers(r)

	r.GET("/internal/cron/read-config", basemw.Extend(gaemiddleware.RequireCron), readConfigCron)
	r.GET("/internal/cron/fetch-crl", basemw.Extend(gaemiddleware.RequireCron), fetchCRLCron)
	r.GET("/internal/cron/bqlog/machine-tokens-flush", basemw.Extend(gaemiddleware.RequireCron), flushMachineTokensLogCron)
	r.GET("/internal/cron/bqlog/delegation-tokens-flush", basemw.Extend(gaemiddleware.RequireCron), flushDelegationTokensLogCron)
	r.GET("/internal/cron/bqlog/oauth-token-grants-flush", basemw.Extend(gaemiddleware.RequireCron), flushOAuthTokenGrantsLogCron)
	r.GET("/internal/cron/bqlog/oauth-tokens-flush", basemw.Extend(gaemiddleware.RequireCron), flushOAuthTokensLogCron)
	r.GET("/internal/cron/bqlog/project-tokens-flush", basemw.Extend(gaemiddleware.RequireCron), flushProjectTokensLogCron)
	r.GET("/internal/cron/bqlog/service-account-tokens-flush", basemw.Extend(gaemiddleware.RequireCron), flushServiceAccountTokensLogCron)

	http.DefaultServeMux.Handle("/", r)
	appengine.Main()
}

// readConfigCron is handler for /internal/cron/read-config GAE cron task.
func readConfigCron(c *router.Context) {
	// Don't override manually imported configs with 'nil' on devserver.
	if info.IsDevAppServer(c.Context) {
		c.Writer.WriteHeader(http.StatusOK)
		return
	}

	// All config fetching callbacks to call in parallel.
	fetchers := []struct {
		name string
		cb   func(context.Context, *empty.Empty) (*admin.ImportedConfigs, error)
	}{
		{"ImportCAConfigs", adminServer.ImportCAConfigs},
		{"ImportDelegationConfigs", adminServer.ImportDelegationConfigs},
		{"ImportServiceAccountsConfigs", adminServer.ImportServiceAccountsConfigs},
		{"ImportProjectIdentityConfigs", adminServer.ImportProjectIdentityConfigs},
		{"ImportProjectOwnedAccountsConfigs", adminServer.ImportProjectOwnedAccountsConfigs},
	}

	errs := make([]error, len(fetchers))

	wg := sync.WaitGroup{}
	wg.Add(len(fetchers))
	for idx, fetcher := range fetchers {
		idx := idx
		fetcher := fetcher
		go func() {
			defer wg.Done()
			if _, errs[idx] = fetcher.cb(c.Context, nil); errs[idx] != nil {
				logging.Errorf(c.Context, "%s failed - %s", fetcher.name, errs[idx])
			}
		}()
	}
	wg.Wait()

	// Retry cron job only on transient errors. On fatal errors let it rerun one
	// minute later, as usual, to avoid spamming logs with errors.
	c.Writer.WriteHeader(statusFromErrs(errs))
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

// flushOAuthTokenGrantsLogCron is handler for /internal/cron/bqlog/oauth-token-grants-flush.
func flushOAuthTokenGrantsLogCron(c *router.Context) {
	// FlushGrantsLog logs errors inside. We also do not retry on errors. It's
	// fine to wait and flush on the next iteration.
	serviceaccounts.FlushGrantsLog(c.Context)
	c.Writer.WriteHeader(http.StatusOK)
}

// flushOAuthTokensLogCron is handler for /internal/cron/bqlog/oauth-tokens-flush.
func flushOAuthTokensLogCron(c *router.Context) {
	// FlushOAuthTokensLog logs errors inside. We also do not retry on errors.
	// It's fine to wait and flush on the next iteration.
	serviceaccounts.FlushOAuthTokensLog(c.Context)
	c.Writer.WriteHeader(http.StatusOK)
}

// flushProjectTokensLogCron is handler for /internal/cron/bqlog/project-tokens-flush.
func flushProjectTokensLogCron(c *router.Context) {
	projectscope.FlushTokenLog(c.Context)
	c.Writer.WriteHeader(http.StatusOK)
}

// flushServiceAccountTokensLogCron is handler for /internal/cron/bqlog/service-account-tokens-flush.
func flushServiceAccountTokensLogCron(c *router.Context) {
	serviceaccountsv2.FlushTokenLog(c.Context)
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
