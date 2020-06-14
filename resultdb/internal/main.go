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

package internal

import (
	"flag"

	"cloud.google.com/go/spanner"
	"golang.org/x/net/context"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/limiter"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/resultdb/internal/span"
)

const (
	// accessGroup is a CIA group that can access ResultDB.
	// TODO(crbug.com/1013316): remove in favor of realms.
	accessGroup = "luci-resultdb-access"
)

// Main runs a service.
//
// Registers -spanner-database flag and initializes a Spanner client.
//
// Registers flags for Redis connection.
func Main(init func(srv *server.Server) error) {
	modules := []module.Module{
		limiter.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
	}

	spannerDB := flag.String("spanner-database", "", "Name of the spanner database to connect to")
	prodMode := flag.Bool("resultdb-prod", false, "Run ResultDB in production mode")

	server.Main(nil, modules, func(srv *server.Server) error {
		var err error
		if srv.Context, err = withProdSpannerClient(srv.Context, *spannerDB, !*prodMode); err != nil {
			return err
		}
		srv.RegisterCleanup(func(ctx context.Context) { span.Client(ctx).Close() })

		return init(srv)
	})
}

// TODO(vadimsh): Move to a module.Module.
func withProdSpannerClient(ctx context.Context, dbFlag string, trackSessionHandles bool) (context.Context, error) {
	if dbFlag == "" {
		return ctx, errors.Reason("-spanner-database flag is required").Err()
	}

	// A token source with Cloud scope.
	ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return ctx, errors.Annotate(err, "failed to get the token source").Err()
	}

	// Init a Spanner client.
	cfg := spanner.ClientConfig{
		SessionPoolConfig: spanner.SessionPoolConfig{
			TrackSessionHandles: trackSessionHandles,
		},
	}
	spannerClient, err := spanner.NewClientWithConfig(ctx, dbFlag, cfg, option.WithTokenSource(ts))
	if err != nil {
		return ctx, err
	}

	// Run a "ping" query to verify the database exists and we can access it
	// before we actually serve any requests. On misconfiguration better to fail
	// early.
	iter := spannerClient.Single().Query(ctx, spanner.NewStatement("SELECT 1;"))
	if err := iter.Do(func(*spanner.Row) error { return nil }); err != nil {
		return ctx, errors.Annotate(err, "failed to ping Spanner").Err()
	}

	return span.WithClient(ctx, spannerClient), nil
}
