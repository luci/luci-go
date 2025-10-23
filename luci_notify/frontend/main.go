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

package main

import (
	"context"
	"net/http"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/config/server/cfgmodule"
	"go.chromium.org/luci/grpc/prpc"
	mailer_pb "go.chromium.org/luci/mailer/api/mailer"
	luciserver "go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/mailer"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/router"
	spanmodule "go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	pb "go.chromium.org/luci/luci_notify/api/service/v1"
	"go.chromium.org/luci/luci_notify/config"
	"go.chromium.org/luci/luci_notify/internal/span"
	"go.chromium.org/luci/luci_notify/notify"
	"go.chromium.org/luci/luci_notify/rpc"
	"go.chromium.org/luci/luci_notify/server"
)

const (
	AccessGroup = "luci-notify-api-access"
)

func checkAPIAccess(ctx context.Context, methodName string, req proto.Message) (context.Context, error) {
	switch yes, err := auth.IsMember(ctx, AccessGroup); {
	case err != nil:
		return nil, status.Errorf(codes.Internal, "error when checking group membership")
	case !yes:
		return nil, status.Errorf(codes.PermissionDenied, "%s does not have access to method %s of Luci Notify", auth.CurrentIdentity(ctx), methodName)
	default:
		return ctx, nil
	}
}

func main() {
	modules := []module.Module{
		cfgmodule.NewModuleFromFlags(),
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
		mailer.NewModuleFromFlags(),
		pubsub.NewModuleFromFlags(),
		spanmodule.NewModuleFromFlags(nil),
		tq.NewModuleFromFlags(),
	}

	notify.InitDispatcher(&tq.Default)

	luciserver.Main(nil, modules, func(srv *luciserver.Server) error {
		// Cron endpoints.
		cron.RegisterHandler("update-config", config.UpdateHandler)
		cron.RegisterHandler("update-tree-status", notify.UpdateTreeStatus)
		cron.RegisterHandler("builder-health-notifier", notify.NotifyOwners)

		// Buildbucket Pub/Sub endpoint.
		pubsub.RegisterJSONPBHandler("buildbucket", notify.BuildbucketPubSubHandler)

		// Install pRPC services.
		srv.ConfigurePRPC(func(s *prpc.Server) {
			s.AccessControl = prpc.AllowOriginAll
			// TODO(crbug/1082369): Remove this workaround once non-standard field
			// masks are no longer used in the API.
			s.EnableNonStandardFieldMasks = true
		})
		srv.RegisterUnaryServerInterceptors(span.SpannerDefaultsInterceptor())

		pb.RegisterTreeCloserServer(srv, &pb.DecoratedTreeCloser{
			Service: &server.TreeCloserServer{},
			Prelude: checkAPIAccess,
		})
		pb.RegisterAlertsServer(srv, rpc.NewAlertsServer())
		pb.RegisterAlertGroupsServer(srv, rpc.NewAlertGroupsServer())
		mailer_pb.RegisterMailerServer(srv, rpc.NewMailerServer())

		// Redirect the frontend to rpcexplorer.
		srv.Routes.GET("/", nil, func(ctx *router.Context) {
			http.Redirect(ctx.Writer, ctx.Request, "/rpcexplorer/", http.StatusFound)
		})
		return nil
	})
}
