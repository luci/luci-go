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
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	tspb "go.chromium.org/luci/tree_status/proto/v1"
)

// Client defines the interface that interacts with Tree status App.
type Client interface {
	// FetchLatest fetches the latest tree status.
	FetchLatest(ctx context.Context, endpoint string) (Status, error)
}

// Status models the status returned by tree status app.
//
// Note that only fields that are needed in CV are included.
// Source of Truth: https://source.chromium.org/chromium/infra/infra/+/52a8cfcb436b0012e668630a2f261237046a033a:appengine/chromium_status/appengine_module/chromium_status/status.py;l=209-252
type Status struct {
	// State describes the Tree state.
	State State
	// Since is the timestamp when the tree obtains the current state.
	Since time.Time
}

// State enumerates possible values for tree state.
type State int8

const (
	StateUnknown State = iota
	Open
	Closed
	Throttled
	InMaintenance
)

func convertToTreeState(s tspb.GeneralState) State {
	switch s {
	case tspb.GeneralState_OPEN:
		return Open
	case tspb.GeneralState_CLOSED:
		return Closed
	case tspb.GeneralState_THROTTLED:
		return Throttled
	case tspb.GeneralState_MAINTENANCE:
		return InMaintenance
	default:
		return StateUnknown
	}
}

func NewClient(ctx context.Context, luciTreeStatusHost string) (Client, error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	rpcOpts := prpc.DefaultOptions()
	rpcOpts.Insecure = lhttp.IsLocalHost(luciTreeStatusHost)
	prpcClient := &prpc.Client{
		C:                     &http.Client{Transport: t},
		Host:                  luciTreeStatusHost,
		Options:               rpcOpts,
		MaxConcurrentRequests: 100,
	}

	return &treeStatusClientImpl{
		client: tspb.NewTreeStatusPRPCClient(prpcClient),
	}, nil
}

type treeStatusClientImpl struct {
	client tspb.TreeStatusClient
}

// FetchLatest fetches the latest tree status.
func (c treeStatusClientImpl) FetchLatest(ctx context.Context, treeName string) (Status, error) {
	response, err := c.client.GetStatus(ctx, &tspb.GetStatusRequest{
		Name: fmt.Sprintf("trees/%s/status/latest", treeName),
	})
	if err != nil {
		return Status{}, errors.Annotate(err, "failed to fetch tree status for %s", treeName).Err()
	}

	return Status{
		State: convertToTreeState(response.GeneralState),
		Since: response.CreateTime.AsTime(),
	}, nil
}

// URLToTreeName converts a tree status endpoint URL to a tree name.
// This follows the convention used to convert the URL in the old tree status apps.
func URLToTreeName(url string) string {
	treeName := strings.TrimPrefix(url, "https://")
	treeName = strings.TrimPrefix(treeName, "http://")
	treeName = strings.TrimSuffix(treeName, "/")
	treeName = strings.TrimSuffix(treeName, ".appspot.com")
	treeName = strings.TrimSuffix(treeName, "-status")
	return treeName
}
