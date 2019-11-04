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
	"net/http"

	"google.golang.org/grpc"

	"cloud.google.com/go/spanner"
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"

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
func Main(init func(srv *server.Server) error) {
	spannerDB := flag.String("spanner-database", "", "Name of the spanner database to connect to")

	server.Main(nil, func(srv *server.Server) error {
		var err error
		if srv.Context, err = withProdSpannerClient(srv.Context, *spannerDB); err != nil {
			return err
		}

		srv.PRPC.UnaryServerInterceptor = commonInterceptor

		return init(srv)
	})
}

func withProdSpannerClient(ctx context.Context, dbFlag string) (context.Context, error) {
	if dbFlag == "" {
		return ctx, errors.Reason("-spanner-database flag is required").Err()
	}

	// Init a Spanner client.
	spannerClient, err := spanner.NewClient(ctx, dbFlag)
	if err != nil {
		return ctx, err
	}
	return span.WithClient(ctx, spannerClient), nil
}

func commonInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
	if err := verifyAccess(ctx); err != nil {
		return nil, err
	}

	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	ctx = WithHTTPClient(ctx, &http.Client{Transport: tr})

	return handler(ctx, req)
}

func verifyAccess(ctx context.Context) error {
	// TODO(crbug.com/1013316): use realms.

	// WARNING: removing this restriction requires changing the way we
	// authenticate to other services: we cannot use AsSelf RPC authority
	// without the restriction defined here.
	switch allowed, err := auth.IsMember(ctx, accessGroup); {
	case err != nil:
		return err

	case !allowed:
		return errors.
			Reason("%s is not in %q CIA group", auth.CurrentIdentity(ctx), accessGroup).
			Tag(grpcutil.PermissionDeniedTag).
			Err()

	default:
		return nil
	}
}
