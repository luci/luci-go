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
package main

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/tokenserver/api/admin/v1"

	"go.chromium.org/luci/tokenserver/appengine/impl"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/bq"
)

func main() {
	impl.Main(func(srv *server.Server, services *impl.Services) error {
		bqInserter, err := bq.NewInserter(srv.Context, srv.Options.CloudProject)
		if err != nil {
			return err
		}

		// GAE crons.
		cron.RegisterHandler("read-config", func(ctx context.Context) error {
			return readConfigCron(ctx, services.Admin)
		})
		cron.RegisterHandler("fetch-crl", func(ctx context.Context) error {
			return fetchCRLCron(ctx, services.Certs)
		})

		// PubSub push handler processing messages produced by impl/utils/bq.
		oidcMW := router.NewMiddlewareChain(
			auth.Authenticate(&openid.GoogleIDTokenAuthMethod{
				AudienceCheck: openid.AudienceMatchesHost,
			}),
		)
		// bigquery-log-pubsub@ is a part of the PubSub Push subscription config.
		pusherID := identity.Identity(fmt.Sprintf("user:bigquery-log-pubsub@%s.iam.gserviceaccount.com", srv.Options.CloudProject))
		srv.Routes.POST("/internal/pubsub/bigquery-log", oidcMW, func(c *router.Context) {
			if got := auth.CurrentIdentity(c.Context); got != pusherID {
				logging.Errorf(c.Context, "Expecting ID token of %q, got %q", pusherID, got)
				c.Writer.WriteHeader(403)
			} else {
				err := bqInserter.HandlePubSubPush(c.Context, c.Request.Body)
				if err != nil {
					logging.Errorf(c.Context, "Failed to process the message: %s", err)
					c.Writer.WriteHeader(500)
				}
			}
		})

		return nil
	})
}

// readConfigCron is a handler for the "read-config" cron.
func readConfigCron(ctx context.Context, adminServer admin.AdminServer) error {
	// All config fetching callbacks to call in parallel.
	fetchers := []struct {
		name string
		cb   func(context.Context, *emptypb.Empty) (*admin.ImportedConfigs, error)
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
			if _, errs[idx] = fetcher.cb(ctx, nil); errs[idx] != nil {
				logging.Errorf(ctx, "%s failed - %s", fetcher.name, errs[idx])
			}
		}()
	}
	wg.Wait()

	// Retry cron job only on transient errors. On fatal errors let it rerun one
	// minute later, as usual, to avoid spamming logs with errors.
	return statusFromErrs(errs)
}

// fetchCRLCron is a handler for the "fetch-crl" cron.
func fetchCRLCron(ctx context.Context, caServer admin.CertificateAuthoritiesServer) error {
	list, err := caServer.ListCAs(ctx, nil)
	if err != nil {
		panic(err) // let the panic catcher deal with it
	}

	// Fetch CRL of each active CA in parallel. In practice there are very few
	// CAs there (~= 1), so the risk of OOM is small.
	wg := sync.WaitGroup{}
	errs := make([]error, len(list.Cn))
	for i, cn := range list.Cn {
		wg.Add(1)
		go func(i int, cn string) {
			defer wg.Done()
			_, err := caServer.FetchCRL(ctx, &admin.FetchCRLRequest{Cn: cn})
			if err != nil {
				logging.Errorf(ctx, "FetchCRL(%q) failed - %s", cn, err)
				errs[i] = err
			}
		}(i, cn)
	}
	wg.Wait()

	// Retry cron job only on transient errors. On fatal errors let it rerun one
	// minute later, as usual, to avoid spamming logs with errors.
	return statusFromErrs(errs)
}

// statusFromErrs returns a transient error if any of gRPC errors is Internal.
func statusFromErrs(errs []error) error {
	var merr errors.MultiError
	var retry bool
	for _, err := range errs {
		if err != nil {
			merr = append(merr, err)
			retry = retry || grpc.Code(err) == codes.Internal
		}
	}
	switch {
	case len(merr) == 0:
		return nil
	case retry:
		return transient.Tag.Apply(merr)
	default:
		return merr
	}
}
