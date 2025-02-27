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

// Package resultdb is a ResultDB client used by the Swarming server.
package resultdb

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
)

// RecorderFactory creates a client to interact with ResultDB Recorder.
type RecorderFactory interface {
	// MakeClient creates a client to interact with ResultDB Recorder.
	MakeClient(ctx context.Context, host string, project string) (*RecorderClient, error)
}

type prodClientFactory struct {
	swarmingProject string
}

// MakeClient implements RecorderFactory.
//
// Creates a client to interact with ResultDB Recorder.
//
// If project is given, the client will act as the project; otherwise act as self.
func (pcf *prodClientFactory) MakeClient(ctx context.Context, host, project string) (*RecorderClient, error) {
	trimmed, err := extractHostname(host)
	if err != nil {
		return nil, err
	}
	var t http.RoundTripper
	if project == "" {
		t, err = auth.GetRPCTransport(ctx, auth.AsSelf)
	} else {
		t, err = auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(project))
	}
	if err != nil {
		return nil, err
	}
	return &RecorderClient{
		client: rdbpb.NewRecorderPRPCClient(
			&prpc.Client{
				C:    &http.Client{Transport: t},
				Host: trimmed,
			}),
		swarmingProject: pcf.swarmingProject,
	}, nil
}

func extractHostname(baseURL string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", errors.Annotate(err, "invalid base URL %q", baseURL).Err()
	}
	return u.Hostname(), nil
}

// NewRecorderFactory returns a RecorderFactory to create
// a Recorder client in production.
func NewRecorderFactory(swarmingProject string) RecorderFactory {
	return &prodClientFactory{swarmingProject: swarmingProject}
}

// RecorderClient is a ResultDB Recorder API wrapper for swarming-specific usage.
//
// In prod, a Recorder for interacting with ResultDB's Recorder service
// will be used. Tests should use a fake implementation.
type RecorderClient struct {
	client          rdbpb.RecorderClient
	swarmingProject string
}

// CreateInvocation creates a ResultDB invocation for the task.
//
// `taskID` should be the task ID as run result (ending with 1).
//
// Returns the created invocation's name and update token. May also return
// a grpc error if something is wrong.
func (recorder *RecorderClient) CreateInvocation(ctx context.Context, taskID, realm string, deadline time.Time) (invName string, updateToken string, err error) {
	req := &rdbpb.CreateInvocationRequest{
		InvocationId: InvocationID(recorder.swarmingProject, taskID),
		Invocation: &rdbpb.Invocation{
			ProducerResource: fmt.Sprintf("//%s/tasks/%s", swarmingHost(recorder.swarmingProject), taskID),
			Realm:            realm,
			Deadline:         timestamppb.New(deadline),
		},
		RequestId: uuid.New().String(),
	}
	header := metadata.MD{}
	inv, err := recorder.client.CreateInvocation(ctx, req, grpc.Header(&header))
	if err != nil {
		return "", "", err
	}
	token, ok := header[rdbpb.UpdateTokenMetadataKey]
	if !ok {
		return "", "", status.Errorf(codes.Internal, "missing update token")
	}
	return inv.Name, token[0], nil
}

// FinalizeInvocation finalized the ResultDB invocation for the task.
func (recorder *RecorderClient) FinalizeInvocation(ctx context.Context, taskID string, updateToken string) error {
	req := &rdbpb.FinalizeInvocationRequest{
		Name: InvocationName(recorder.swarmingProject, taskID),
	}
	ctx = metadata.AppendToOutgoingContext(ctx, rdbpb.UpdateTokenMetadataKey, updateToken)
	_, err := recorder.client.FinalizeInvocation(ctx, req)
	return err
}

// InvocationName returns the task's ResultDB invocation name.
func InvocationName(swarmingProject, taskID string) string {
	return fmt.Sprintf("invocations/%s", InvocationID(swarmingProject, taskID))
}

// InvocationID returns the task's ResultDB invocation ID.
func InvocationID(swarmingProject, taskID string) string {
	return fmt.Sprintf("task-%s-%s", swarmingHost(swarmingProject), taskID)
}

func swarmingHost(swarmingProject string) string {
	return fmt.Sprintf("%s.appspot.com", swarmingProject)
}
