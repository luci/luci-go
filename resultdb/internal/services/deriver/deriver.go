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

package deriver

import (
	"time"

	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// deriverServer implements pb.DeriverServer.
//
// It does not return gRPC-native errors; use DecoratedDeriver with
// internal.CommonPostlude.
type deriverServer struct {
	*Options
}

// Options is deriver server configuration.
type Options struct {
	// BigQuery table that the derived invocations should be exported to.
	InvBQTable *pb.BigQueryExport

	// Duration since invocation creation after which to delete expected test
	// results.
	ExpectedResultsExpiration time.Duration
}

// InitServer initializes a deriver server.
func InitServer(srv *server.Server, opt Options) {
	pb.RegisterDeriverServer(srv.PRPC, &pb.DecoratedDeriver{
		Service:  &deriverServer{Options: &opt},
		Postlude: internal.CommonPostlude,
	})
}
