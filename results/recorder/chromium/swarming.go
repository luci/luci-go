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

package chromium

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
)

const (
	outputJSONFilename  = "output.json"
	swarmingAPIEndpoint = "_ah/api/swarming/v1/"
)

// SwarmingService provides an interface for specifying how to talk to swarming services.
type SwarmingService interface {
	// GetHost returns the host associated with this service.
	GetHost() string

	// GetTaskResult returns the result of the provided task.
	GetTaskResult(ctx context.Context, taskID string) (*swarming.SwarmingRpcsTaskResult, error)
}

// FetchOutputJSON fetches the output.json from the given task on the given host.
//
// TODO: convert the bytes.Buffer to a resultspb.Invocation.
func FetchOutputJSON(ctx context.Context, swarmSvc SwarmingService, id string) (*bytes.Buffer, error) {
	// Get the ref for the isolated outs of the given task.
	ref, err := GetOutputsRef(ctx, swarmSvc, id)
	if err != nil {
		return nil, err
	}

	host := swarmSvc.GetHost()
	isoClient, err := getProdIsolatedClient(ctx, ref)
	if err != nil {
		return nil, err
	}

	// Fetch the isolate.
	logging.Infof(
		ctx, "Fetching %s for isolated outs of task %s in %s", string(ref.Isolated), id, host)
	buf := &bytes.Buffer{}
	if err := isoClient.Fetch(ctx, isolated.HexDigest(ref.Isolated), buf); err != nil {
		return nil, err
	}

	isolates := &isolated.Isolated{}
	json.Unmarshal(buf.Bytes(), isolates)

	outputFile, ok := isolates.Files[outputJSONFilename]
	if !ok {
		return nil, errors.Reason(
			"Missing expected output %s in isolated outputs of task %s in %s",
			outputJSONFilename, id, host).Err()
	}

	// Now fetch (from the same server and namespace) the output file by digest.
	logging.Infof(ctx, "Fetching %s for output of task %s in %s", string(outputFile.Digest), id, host)
	buf = &bytes.Buffer{}
	if err := isoClient.Fetch(ctx, outputFile.Digest, buf); err != nil {
		return nil, err
	}

	// TODO: Get the right format for buffer conversion and convert accordingly.

	return buf, nil
}

// GetOutputsRef gets the data needed to fetch the isolated outputs given a task and swarming host.
func GetOutputsRef(ctx context.Context, swarmSvc SwarmingService, id string) (*swarming.SwarmingRpcsFilesRef, error) {
	logging.Infof(ctx, "Fetching outputs ref for task %s in %s", id, swarmSvc.GetHost())
	taskResult, err := swarmSvc.GetTaskResult(ctx, id)
	if err != nil {
		return nil, err
	}

	if taskResult.OutputsRef == nil || taskResult.OutputsRef.Isolated == "" {
		return nil, errors.Reason("Task %s in %s had no isolated outputs", id, swarmSvc.GetHost()).Err()
	}

	return taskResult.OutputsRef, nil
}

// ProdSwarmingService defines an implementation of SwarmingService to use in prod.
type ProdSwarmingService struct {
	// host is the swarming host.
	host string

	// Service is a pointer to the prod swarming.Service to use.
	Service *swarming.Service
}

// GetProdSwarmingService returns a SwarmingService that talks to prod swarming.
func GetProdSwarmingService(ctx context.Context, host string) (*ProdSwarmingService, error) {
	service, err := swarming.NewService(ctx)
	if err != nil {
		return nil, err
	}

	service.BasePath = fmt.Sprintf("%s/%s", host, swarmingAPIEndpoint)
	return &ProdSwarmingService{host, service}, nil
}

// GetHost returns basepath for this prod swarming service.
func (s *ProdSwarmingService) GetHost() string {
	return s.host
}

// GetTaskResult gets the task with given ID.
func (s *ProdSwarmingService) GetTaskResult(ctx context.Context, taskID string) (*swarming.SwarmingRpcsTaskResult, error) {
	resultCall := s.Service.Task.Result(taskID)
	resultCall = resultCall.Context(ctx)
	return resultCall.Do()
}

// getProdIsolatedClient gets the correct prod isolateserver for the given object ref.
// TODO: handle the authClient correctly.
func getProdIsolatedClient(ctx context.Context, ref *swarming.SwarmingRpcsFilesRef) (*isolatedclient.Client, error) {
	authTransport, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}

	return isolatedclient.New(
		nil,
		&http.Client{Transport: authTransport},
		ref.Isolatedserver,
		ref.Namespace,
		nil,
		nil,
	), nil
}
