// Copyright 2022 The LUCI Authors.
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
	"context"
	"time"

	"cloud.google.com/go/pubsub"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	glistener "go.chromium.org/luci/cv/internal/gerrit/listener"
)

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		redisconn.NewModuleFromFlags(),
		secrets.NewModuleFromFlags(),
		tq.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		creds, err := auth.GetPerRPCCredentials(
			srv.Context, auth.AsSelf,
			auth.WithScopes(auth.CloudOAuthScopes...),
		)
		if err != nil {
			return errors.Annotate(err, "failed to get per RPC credentials").Err()
		}
		psc, err := pubsub.NewClient(
			srv.Context, srv.Options.CloudProject,
			option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
			option.WithGRPCDialOption(grpc.WithStatsHandler(otelgrpc.NewClientHandler())),
			option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
		)
		if err != nil {
			return errors.Annotate(err, "pubsub.NewClient: %s", err).Err()
		}
		clUpdater := changelist.NewUpdater(&tq.Default, nil)
		gListener := glistener.NewListener(psc, clUpdater)
		srv.RunInBackground("luci.cv.listener.gerrit_subscriptions", gListener.Run)

		cron.RegisterHandler("refresh-listener-config", func(ctx context.Context) error {
			return refreshConfig(ctx)
		})
		return nil
	})
}

func refreshConfig(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()
	return srvcfg.ImportConfig(ctx)
}
