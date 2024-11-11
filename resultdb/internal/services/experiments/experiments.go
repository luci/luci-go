// Copyright 2024 The LUCI Authors.
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

// Package experiments provides a place to experiment within ResultDB
// while being (mostly) isolated from production services.
package experiments

import (
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// experimentsServer implements pb.ExperimentsServer.
//
// It does not return gRPC-native errors; use DecoratedExperiments with
// internal.CommonPostlude.
type experimentsServer struct {
	// Add any clients you require that benefit from connection pooling,
	// e.g. BigQuery clients.
	// For Spanner, the client is already installed in server context
	// and accessible via "go.chromium.org/luci/server/span" module
	// so we do not need to create a client.
}

// InitServer initializes a recorder server.
func InitServer(srv *server.Server) error {
	pb.RegisterExperimentsServer(srv, NewExperimentsServer())

	// TODO(crbug/1082369): Remove this workaround once field masks can be decoded.
	srv.ConfigurePRPC(func(p *prpc.Server) {
		p.EnableNonStandardFieldMasks = true
	})

	// Experiments run with medium priority, not high, to avoid starving more
	// important jobs.
	srv.RegisterUnaryServerInterceptors(spanutil.SpannerDefaultsInterceptor(sppb.RequestOptions_PRIORITY_MEDIUM))
	return nil
}

func NewExperimentsServer() *pb.DecoratedExperiments {
	return &pb.DecoratedExperiments{
		Service:  &experimentsServer{},
		Postlude: internal.CommonPostlude,
	}
}
