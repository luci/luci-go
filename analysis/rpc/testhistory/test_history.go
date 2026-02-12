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

package testhistory

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/testrealms"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/analysis/rpc"
)

func init() {
	rdbperms.PermListTestResults.AddFlags(realms.UsedInQueryRealms)
	rdbperms.PermListTestExonerations.AddFlags(realms.UsedInQueryRealms)
}

type TestSearchClient interface {
	QueryTests(ctx context.Context, project, testIDSubstring string, opts testrealms.QueryTestsOptions) (testIDs []string, nextPageToken string, err error)
}

// testHistoryServer implements pb.TestHistoryServer.
type testHistoryServer struct {
	searchClient TestSearchClient
}

// NewTestHistoryServer returns a new pb.TestHistoryServer.
func NewTestHistoryServer(searchClient TestSearchClient) pb.TestHistoryServer {
	return &pb.DecoratedTestHistory{
		Service:  &testHistoryServer{searchClient: searchClient},
		Postlude: rpc.GRPCifyAndLogPostlude,
	}
}

func queryPreviousTestIDFromResultDB(ctx context.Context, project, testID string) (string, error) {
	cl, err := resultdb.NewCredentialForwardingClient(ctx, chromeinfra.ResultDBHost)
	if err != nil {
		return "", err
	}

	req := &resultpb.QueryTestMetadataRequest{
		Project: project,
		Predicate: &resultpb.TestMetadataPredicate{
			TestIds: []string{testID},
		},
	}
	rsp, err := cl.QueryTestMetadata(ctx, req)
	if err != nil {
		code := status.Code(err)
		if code == codes.PermissionDenied {
			return "", appstatus.Errorf(codes.PermissionDenied, "caller does not have permission to query for previous test ID: %s", err)
		}
		return "", fmt.Errorf("query previous test ID from ResultDB: %w", err)
	}

	if len(rsp.TestMetadata) == 0 {
		return "", nil
	}
	const mainGitRef = "refs/heads/main"

	// Default to the first item.
	metadata := rsp.TestMetadata[0]
	for _, md := range rsp.TestMetadata {
		// Prefer something from a main branch, if there is one.
		if md.SourceRef.GetGitiles().GetRef() == mainGitRef {
			metadata = md
			break
		}
	}
	return metadata.TestMetadata.GetPreviousTestId(), nil
}
