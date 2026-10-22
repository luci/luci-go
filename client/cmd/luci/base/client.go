// Copyright 2026 The LUCI Authors.
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

package base

import (
	"context"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/prpc"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// NewResultDBClient creates authenticated ResultDB and Schemas pRPC clients.
func (af *AuthFlags) NewResultDBClient(ctx context.Context, host string) (pb.ResultDBClient, pb.SchemasClient, *http.Client, error) {
	httpClient, err := af.NewHTTPClient(ctx)
	if err != nil {
		return nil, nil, nil, errors.Fmt("failed to create http client: %w", err)
	}
	prpcClient := &prpc.Client{
		C:       httpClient,
		Host:    host,
		Options: prpc.DefaultOptions(),
	}
	return pb.NewResultDBClient(prpcClient), pb.NewSchemasClient(prpcClient), httpClient, nil
}

// NewBuildsClient creates an authenticated Buildbucket Builds pRPC client.
func (af *AuthFlags) NewBuildsClient(ctx context.Context, host string) (grpcpb.BuildsClient, *http.Client, error) {
	httpClient, err := af.NewHTTPClient(ctx)
	if err != nil {
		return nil, nil, errors.Fmt("failed to create http client: %w", err)
	}
	prpcClient := &prpc.Client{
		C:       httpClient,
		Host:    host,
		Options: prpc.DefaultOptions(),
	}
	return grpcpb.NewBuildsClient(prpcClient), httpClient, nil
}

// NewLogDogClient creates an authenticated LogDog Logs pRPC client.
func (af *AuthFlags) NewLogDogClient(ctx context.Context, host string) (logdog.LogsClient, error) {
	httpClient, err := af.NewHTTPClient(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to create http client: %w", err)
	}
	prpcClient := &prpc.Client{
		C:       httpClient,
		Host:    host,
		Options: prpc.DefaultOptions(),
	}
	return logdog.NewLogsClient(prpcClient), nil
}

// NewAnalysisClients creates authenticated LUCI Analysis Rules, Clusters, and TestHistory
// pRPC clients sharing a single underlying HTTP transport against host (typically
// chromeinfra.AnalysisHost, i.e. "analysis.api.luci.app").
func (af *AuthFlags) NewAnalysisClients(ctx context.Context, host string) (analysispb.RulesClient, analysispb.ClustersClient, analysispb.TestHistoryClient, error) {
	httpClient, err := af.NewHTTPClient(ctx)
	if err != nil {
		return nil, nil, nil, errors.Fmt("failed to create http client: %w", err)
	}
	prpcClient := &prpc.Client{
		C:       httpClient,
		Host:    host,
		Options: prpc.DefaultOptions(),
	}
	return analysispb.NewRulesClient(prpcClient), analysispb.NewClustersClient(prpcClient), analysispb.NewTestHistoryClient(prpcClient), nil
}

// IsLegacyInvocation queries ResultDB directly to determine if an invocation is a legacy invocation
// (e.g. Fuchsia/Turquoise builds not yet migrated to ResultDB Root Invocations).
// It queries for the root invocation, and if not found, queries for the legacy invocation.
// It suppresses pRPC's Warning log when GetRootInvocation returns an expected NotFound on legacy builds.
func IsLegacyInvocation(ctx context.Context, client pb.ResultDBClient, invID string) bool {
	normalized := NormalizeInvocation(invID)
	if client == nil || normalized == "" {
		return false
	}
	probeCtx := logging.SetLevel(ctx, logging.Error)
	_, err := client.GetRootInvocation(probeCtx, &pb.GetRootInvocationRequest{
		Name: "rootInvocations/" + normalized,
	})
	if err == nil {
		return false
	}
	code := status.Code(err)
	if code == codes.Unknown {
		code = appstatus.Code(err)
	}
	if code == codes.NotFound {
		_, errLegacy := client.GetInvocation(probeCtx, &pb.GetInvocationRequest{
			Name: "invocations/" + normalized,
		})
		if errLegacy == nil {
			return true
		}
	}
	return false
}
