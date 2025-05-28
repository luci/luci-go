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

// Package resultdb provides implementation for luci.resultdb.v1.ResultDB
// service.
package resultdb

import (
	"context"

	"cloud.google.com/go/bigquery"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/genproto/googleapis/bytestream"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gerritauth"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/ensureviews"
	"go.chromium.org/luci/resultdb/internal/rpcutil"
	"go.chromium.org/luci/resultdb/internal/services/resultdb/schema"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// resultDBServer implements pb.ResultDBServer.
//
// It does not return gRPC-native errors; use DecoratedResultDB with
// internal.CommonPostlude.
type resultDBServer struct {
	contentServer    *artifactcontent.Server
	bqClient         *bigquery.Client
	artifactBQClient artifacts.BQClient
}

// Options is resultdb server configuration.
type Options struct {
	// InsecureSelfURLs is set to true to use http:// (not https://) for URLs
	// pointing back to ResultDB.
	InsecureSelfURLs bool

	// ContentHostnameMap maps a Host header of GetArtifact request to a host name
	// to use for all user-content URLs.
	//
	// Special key "*" indicates a fallback.
	ContentHostnameMap map[string]string

	// ArtifactRBEInstance is the name of the RBE instance to use for artifact
	// storage. Example: "projects/luci-resultdb/instances/artifacts".
	ArtifactRBEInstance string
}

// InitServer initializes a resultdb server.
func InitServer(srv *server.Server, opts Options) error {
	contentServer, err := newArtifactContentServer(srv.Context, opts)
	if err != nil {
		return errors.Fmt("failed to create an artifact content server: %w", err)
	}

	// Serve all possible content hostnames.
	hosts := stringset.New(len(opts.ContentHostnameMap))
	for _, v := range opts.ContentHostnameMap {
		hosts.Add(v)
	}
	for _, host := range hosts.ToSortedSlice() {
		contentServer.InstallHandlers(srv.VirtualHost(host))
	}

	bqClient, err := bq.NewClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Fmt("creating BQ client: %w", err)
	}

	artifactBQClient, err := artifacts.NewClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Fmt("creating artifact BQ client: %w", err)
	}

	// Serve cron jobs endpoints.
	cron.RegisterHandler("read-config", config.UpdateConfig)
	cron.RegisterHandler("ensure-views", func(ctx context.Context) error {
		return ensureviews.CronHandler(ctx, srv.Options.CloudProject)
	})

	rdbSvr := &resultDBServer{
		contentServer:    contentServer,
		bqClient:         bqClient,
		artifactBQClient: artifactBQClient,
	}
	pb.RegisterResultDBServer(srv, &pb.DecoratedResultDB{
		Service:  rdbSvr,
		Postlude: internal.CommonPostlude,
	})
	pb.RegisterSchemasServer(srv, schema.NewSchemasServer())

	// Register an empty Recorder server only to make the discovery service
	// list it.
	// The actual traffic will be directed to another deployment, i.e. this
	// binary will never see Recorder RPCs.
	// TODO(nodir): replace this hack with a separate discovery Deployment that
	// dynamically fetches discovery documents from other deployments and
	// returns their union.
	pb.RegisterRecorderServer(srv, nil)

	srv.ConfigurePRPC(func(p *prpc.Server) {
		// Allow cross-origin calls, in particular calls using Gerrit auth headers.
		p.AccessControl = func(context.Context, string) prpc.AccessControlDecision {
			return prpc.AccessControlDecision{
				AllowCrossOriginRequests: true,
				AllowCredentials:         true,
				AllowHeaders:             []string{gerritauth.Method.Header},
			}
		}
		// TODO(crbug/1082369): Remove this workaround once non-standard field masks
		// are no longer used in the API.
		p.EnableNonStandardFieldMasks = true
	})

	srv.RegisterUnaryServerInterceptors(
		spanutil.SpannerDefaultsInterceptor(sppb.RequestOptions_PRIORITY_MEDIUM),
		rpcutil.IdentityKindCountingInterceptor(),
		rpcutil.RequestTimeoutInterceptor(),
	)
	return nil
}

func newArtifactContentServer(ctx context.Context, opts Options) (*artifactcontent.Server, error) {
	if opts.ArtifactRBEInstance == "" {
		return nil, errors.New("opts.ArtifactRBEInstance is required")
	}

	conn, err := artifactcontent.RBEConn(ctx)
	if err != nil {
		return nil, err
	}
	bs := bytestream.NewByteStreamClient(conn)

	return &artifactcontent.Server{
		InsecureURLs: opts.InsecureSelfURLs,
		HostnameProvider: func(requestHost string) string {
			if host, ok := opts.ContentHostnameMap[requestHost]; ok {
				return host
			}
			return opts.ContentHostnameMap["*"]
		},

		ReadCASBlob: func(ctx context.Context, req *bytestream.ReadRequest) (bytestream.ByteStream_ReadClient, error) {
			return bs.Read(ctx, req)
		},
		RBECASInstanceName: opts.ArtifactRBEInstance,
	}, nil
}
