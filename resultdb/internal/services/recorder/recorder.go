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

package recorder

import (
	"context"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/services/artifactexporter"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// recorderServer implements pb.RecorderServer.
//
// It does not return gRPC-native errors; use DecoratedRecorder with
// internal.CommonPostlude.
type recorderServer struct {
	*Options

	// casClient is an instance of ContentAddressableStorageClient which is used for
	// artifact batch operations.
	casClient repb.ContentAddressableStorageClient

	// bqExportClient is used for exporting artifacts to BigQuery.
	bqExportClient BQExportClient
}

// Options is recorder server configuration.
type Options struct {
	// Duration since invocation creation after which to delete expected test
	// results.
	ExpectedResultsExpiration time.Duration

	// ArtifactRBEInstance is the name of the RBE instance to use for artifact
	// storage. Example: "projects/luci-resultdb/instances/artifacts".
	ArtifactRBEInstance string

	// MaxArtifactContentStreamLength is the maximum size of an artifact content
	// to accept via streaming API.
	MaxArtifactContentStreamLength int64
}

// InitServer initializes a recorder server.
func InitServer(srv *server.Server, opt Options) error {
	conn, err := artifactcontent.RBEConn(srv.Context)
	if err != nil {
		return err
	}
	bqClient, err := artifactexporter.NewClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Annotate(err, "create bq export client").Err()
	}
	srv.RegisterCleanup(func(ctx context.Context) {
		err := bqClient.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up BigQuery export client: %s", err)
		}
	})

	pb.RegisterRecorderServer(srv, &pb.DecoratedRecorder{
		Service: &recorderServer{
			Options:        &opt,
			casClient:      repb.NewContentAddressableStorageClient(conn),
			bqExportClient: bqClient,
		},
		Postlude: internal.CommonPostlude,
	})

	// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
	srv.ConfigurePRPC(func(p *prpc.Server) {
		p.HackFixFieldMasksForJSON = true
	})

	srv.RegisterUnaryServerInterceptors(spanutil.SpannerDefaultsInterceptor(sppb.RequestOptions_PRIORITY_HIGH))
	return installArtifactCreationHandler(srv, &opt, conn)
}

// installArtifactCreationHandler installs artifact creation handler.
func installArtifactCreationHandler(srv *server.Server, opt *Options, rbeConn *grpc.ClientConn) error {
	if opt.ArtifactRBEInstance == "" {
		return errors.Reason("opt.ArtifactRBEInstance is required").Err()
	}

	bs := bytestream.NewByteStreamClient(rbeConn)
	ach := &artifactCreationHandler{
		RBEInstance: opt.ArtifactRBEInstance,
		NewCASWriter: func(ctx context.Context) (bytestream.ByteStream_WriteClient, error) {
			return bs.Write(ctx)
		},
		MaxArtifactContentStreamLength: opt.MaxArtifactContentStreamLength,
	}

	// Ideally we define more specific routes, but
	// "github.com/julienschmidt/httprouter" does not support routes over
	// unescaped paths: https://github.com/julienschmidt/httprouter/issues/208
	srv.Routes.PUT("invocations/*rest", nil, ach.Handle)
	return nil
}
