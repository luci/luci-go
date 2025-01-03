// Copyright 2021 The LUCI Authors.
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

// Package tree implements fetching tree status from Tree Status App.
package tree

import (
	"context"
	"net/http"
	"strings"

	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	tspb "go.chromium.org/luci/tree_status/proto/v1"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
)

// ClientFactory creates client for tree status server that ties to a LUCI project.
type ClientFactory interface {
	MakeClient(ctx context.Context, luciProject string) (tspb.TreeStatusClient, error)
}

// NewClientFactory returns a ClientFactory instance.
func NewClientFactory(host string) ClientFactory {
	return &prpcClientFactory{
		host: host,
	}
}

type prpcClientFactory struct {
	host string
}

// MakeClient implements `ClientFactory`
func (f *prpcClientFactory) MakeClient(ctx context.Context, luciProject string) (tspb.TreeStatusClient, error) {
	prpcClient := &prpc.Client{
		Host: f.host,
	}
	if lhttp.IsLocalHost(f.host) { // testing
		prpcClient.Options = &prpc.Options{
			Retry:    retry.None,
			Insecure: true,
		}
	} else {
		rt, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(luciProject))
		if err != nil {
			return nil, err
		}
		prpcClient.C = &http.Client{Transport: rt}
	}
	return tspb.NewTreeStatusPRPCClient(prpcClient), nil
}

// TreeName extracts tree name from the config.
//
// Converts a tree status endpoint URL to a tree name if only the URL is
// specified
func TreeName(config *cfgpb.Verifiers_TreeStatus) string {
	switch {
	case config.GetTreeName() != "":
		return config.GetTreeName()
	case config.GetUrl() != "":
		treeName := strings.TrimPrefix(config.GetUrl(), "https://")
		treeName = strings.TrimPrefix(treeName, "http://")
		treeName = strings.TrimSuffix(treeName, "/")
		treeName = strings.TrimSuffix(treeName, ".appspot.com")
		treeName = strings.TrimSuffix(treeName, "-status")
		return treeName
	default:
		return ""
	}
}
