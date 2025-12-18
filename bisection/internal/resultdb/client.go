// Copyright 2025 The LUCI Authors.
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

package resultdb

import (
	"context"
	"io"
	"net/http"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/resultdb/pbutil"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
)

//go:generate mockgen -source=client.go -destination=testutil.go -package=resultdb

// Client provides methods to interact with ResultDB.
type Client interface {
	FetchSpecificArtifacts(ctx context.Context, invocationID, testID, resultID string, artifactIDs []string) (map[string]string, error)
}

type clientImpl struct {
	rdbClient  resultpb.ResultDBClient
	httpClient *http.Client
}

// NewClient creates a new ResultDB client.
// project is the LUCI project ID (e.g., "luci-bisection" or "luci-bisection-dev").
func NewClient(ctx context.Context, host string, project string) (Client, error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(project))
	if err != nil {
		return nil, errors.Fmt("failed to get RPC transport: %w", err)
	}

	httpClient := &http.Client{Transport: t}
	prpcClient := &prpc.Client{
		C:    httpClient,
		Host: host,
	}
	rdbClient := resultpb.NewResultDBPRPCClient(prpcClient)

	return &clientImpl{
		rdbClient:  rdbClient,
		httpClient: httpClient,
	}, nil
}

// FetchSpecificArtifacts fetches the content of specific artifacts by their IDs.
func (c *clientImpl) FetchSpecificArtifacts(ctx context.Context, invocationID, testID, resultID string, artifactIDs []string) (map[string]string, error) {
	if len(artifactIDs) == 0 {
		return make(map[string]string), nil
	}

	artifactContents := make(map[string]string)
	parent := pbutil.LegacyTestResultName(invocationID, testID, resultID)

	for _, artifactID := range artifactIDs {
		// Construct the artifact name
		artifactName := parent + "/artifacts/" + artifactID

		// Get the artifact
		req := &resultpb.GetArtifactRequest{
			Name: artifactName,
		}

		artifact, err := c.rdbClient.GetArtifact(ctx, req)
		if err != nil {
			logging.Warningf(ctx, "Failed to get artifact %s: %v", artifactID, err)
			continue
		}

		// Fetch the artifact content
		content, err := c.fetchArtifactContentByURL(ctx, artifact.FetchUrl)
		if err != nil {
			logging.Warningf(ctx, "Failed to fetch content for artifact %s: %v", artifactID, err)
			continue
		}

		artifactContents[artifactID] = content
	}

	return artifactContents, nil
}

// fetchArtifactContentByURL downloads the artifact content from the fetch URL.
func (c *clientImpl) fetchArtifactContentByURL(ctx context.Context, fetchURL string) (string, error) {
	if fetchURL == "" {
		return "", errors.New("empty fetch URL")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
	if err != nil {
		return "", errors.Fmt("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errors.Fmt("failed to fetch artifact: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Fmt("unexpected status code: %d", resp.StatusCode)
	}

	// Read the content with a size limit (e.g., 10MB)
	const maxSize = 10 * 1024 * 1024
	limitedReader := io.LimitReader(resp.Body, maxSize)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", errors.Fmt("failed to read artifact content: %w", err)
	}

	return string(content), nil
}
