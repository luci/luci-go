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

	swarmingAPI "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/logging"
)

const (
	outputJSONFileName  = "output.json"
	swarmingAPIEndpoint = "_ah/api/swarming/v1/"
)

// fetchOutputJSON fetches the output.json from the given task on the given host.
//
// TODO: convert the bytes.Buffer to a resultspb.Invocation.
func fetchOutputJSON(ctx context.Context, cl *http.Client, swarmingURL, taskID string) ([]byte, error) {
	// Set up swarming service for getting task info.
	swarmSvc, err := swarmingAPI.New(cl)
	if err != nil {
		return nil, err
	}
	swarmSvc.BasePath = fmt.Sprintf("%s/%s", swarmingURL, swarmingAPIEndpoint)

	// Get the ref for the isolated outs of the given task.
	ref, err := getOutputsRef(ctx, swarmSvc, taskID)
	if err != nil {
		return nil, err
	}

	// Get isolated client for getting isolated objects.
	isoClient := isolatedclient.New(nil, cl, ref.Isolatedserver, ref.Namespace, nil, nil)

	// Fetch the isolate.
	logging.Infof(
		ctx, "Fetching %s for isolated outs of task %s in %s", ref.Isolated, taskID, swarmingURL)
	buf := &bytes.Buffer{}
	if err := isoClient.Fetch(ctx, isolated.HexDigest(ref.Isolated), buf); err != nil {
		return nil, err
	}

	isolates := &isolated.Isolated{}
	if err := json.Unmarshal(buf.Bytes(), isolates); err != nil {
		return nil, err
	}

	outputFile, ok := isolates.Files[outputJSONFileName]
	if !ok {
		return nil, errors.Reason(
			"missing expected output %s in isolated outputs of task %s in %s",
			outputJSONFileName, taskID, swarmingURL).Err()
	}

	// Now fetch (from the same server and namespace) the output file by digest.
	logging.Infof(
		ctx, "Fetching %s for output of task %s in %s", outputFile.Digest, taskID, swarmingURL)
	buf = &bytes.Buffer{}
	if err := isoClient.Fetch(ctx, outputFile.Digest, buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// getOutputsRef gets the data needed to fetch the isolated outputs given a task and swarming
// service.
func getOutputsRef(ctx context.Context, swarmSvc *swarmingAPI.Service, taskID string) (*swarmingAPI.SwarmingRpcsFilesRef, error) {
	logging.Infof(ctx, "Fetching outputs ref for task %s (%s)", taskID, swarmSvc.BasePath)
	taskResult, err := swarmSvc.Task.Result(taskID).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	if taskResult.OutputsRef == nil || taskResult.OutputsRef.Isolated == "" {
		return nil, errors.Reason("Task %s (%s) had no isolated outputs", taskID, swarmSvc.BasePath).Err()
	}

	return taskResult.OutputsRef, nil
}
