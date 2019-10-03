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

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/recorder/chromium/formats"
	"go.chromium.org/luci/resultdb/recorder/spanner"
)

const (
	outputJSONFileName  = "output.json"
	swarmingAPIEndpoint = "_ah/api/swarming/v1/"
)

// WriteInvocation writes the invocation associated with the given swarming task.
//
// If the task is a dedup of another task, the invocation returned is the underlying one; otherwise,
// the invocation returned is associated with the swarming task itself.
func WriteInvocation(ctx context.Context, req *pb.DeriveInvocationRequest, cl *http.Client, swarmingURL, taskID string, spannerCl *spanner.Client) (*pb.Invocation, error) {
	swarmSvc, err := getSwarmSvc(cl, swarmingURL)
	if err != nil {
		return nil, err
	}

	// Get the swarming task.
	task, err := swarmSvc.Task.Result(taskID).Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	// Check if we even need to write this invocation: is it finalized?
	invID, err := getInvocationId(ctx, task, swarmSvc, swarmingURL)
	if err != nil {
		return nil, err
	}

	if inv, err := spannerCl.GetInvocation(ctx, invID); err != nil {
		return nil, err
	} else if inv != nil && pbutil.IsFinal(inv.State) {
		return inv, nil
	}

	// We may need to write the invocation, so fetch the output JSON.
	buf, err := fetchOutputJSON(ctx, task, cl)
	if err != nil {
		return nil, errors.Annotate(err, "task %s (%s)", taskID, swarmingURL).Err()
	}

	return convertOutputJSON(ctx, req, spannerCl, invID, buf)
}

func getSwarmSvc(cl *http.Client, swarmingURL string) (*swarmingAPI.Service, error) {
	swarmSvc, err := swarmingAPI.New(cl)
	if err != nil {
		return nil, err
	}

	swarmSvc.BasePath = fmt.Sprintf("%s/%s", swarmingURL, swarmingAPIEndpoint)
	return swarmSvc, nil
}

// fetchOutputJSON fetches the output.json from the given task on the given host.
func fetchOutputJSON(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, cl *http.Client) ([]byte, error) {
	// Get the ref for the isolated outs of the given task.
	ref, err := getOutputsRef(ctx, task)
	if err != nil {
		return nil, err
	}

	// Get isolated client for getting isolated objects.
	isoClient := isolatedclient.New(nil, cl, ref.Isolatedserver, ref.Namespace, nil, nil)

	// Fetch the isolate.
	logging.Infof(
		ctx, "Fetching %s for isolated outs of task %s", ref.Isolated, task.TaskId)
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
			"missing expected output %s in isolated outputs",
			outputJSONFileName).Err()
	}

	// Now fetch (from the same server and namespace) the output file by digest.
	logging.Infof(
		ctx, "Fetching %s for output of task %s", outputFile.Digest, task.TaskId)
	buf = &bytes.Buffer{}
	if err := isoClient.Fetch(ctx, outputFile.Digest, buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// getOutputsRef gets the data needed to fetch the isolated outputs given a task and swarming
// service.
func getOutputsRef(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult) (*swarmingAPI.SwarmingRpcsFilesRef, error) {
	if task.OutputsRef == nil || task.OutputsRef.Isolated == "" {
		return nil, errors.Reason("no isolated outputs").Err()
	}

	return task.OutputsRef, nil
}

// getInvocationId gets the ID of the invocation associated with a task and swarming service.
func getInvocationId(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, swarmSvc *swarmingAPI.Service, swarmingURL string) (string, error) {
	if task == nil {
		return "", errors.Reason("cannot get Invocation ID for nil task").Err()
	}

	// If the task was deduped, then the invocation associated with it is just the one associated
	// to the task from which it was deduped.
	if task.DedupedFrom != "" {
		dedupedFromTask, err := swarmSvc.Task.Result(task.DedupedFrom).Context(ctx).Do()
		if err != nil {
			return "", err
		}
		return getInvocationId(ctx, dedupedFromTask, swarmSvc, swarmingURL)
	}

	// Otherwise, use the hostname and task run ID.
	return fmt.Sprintf("%s/%s", swarmingURL, task.RunId), nil
}

func convertOutputJSON(ctx context.Context, req *pb.DeriveInvocationRequest, spannerCl *spanner.Client, invID string, buf []byte) (*pb.Invocation, error) {
	var results []*pb.TestResult
	var err error
	inv := &pb.Invocation{}
	reader := bytes.NewReader(buf)

	// Try to convert the buffer treating its format as the JSON Test Results Format.
	jsonFormat := &formats.JSONTestResults{}
	if jsonErr := jsonFormat.ConvertFromJSON(ctx, reader); jsonErr != nil {
		// Try to convert the buffer treating its format as that of GTests.
		gtestFormat := &formats.GTestResults{}
		reader.Reset(buf)
		if gtestErr := gtestFormat.ConvertFromJSON(ctx, reader); gtestErr != nil {
			return nil, errors.NewMultiError(jsonErr, gtestErr)
		}

		results, err = gtestFormat.ToProtos(ctx, req, inv)
		if err != nil {
			return nil, errors.Annotate(err, "converting as GTest results format").Err()
		}
	} else {
		results, err = jsonFormat.ToProtos(ctx, req, inv)
		if err != nil {
			return nil, errors.Annotate(err, "converting as JSON Test Results Format").Err()
		}
	}

	// Got invocation and est results, so write them.
	if err := spannerCl.WriteTestResults(ctx, invID, inv, results); err != nil {
		return nil, err
	}
	return inv, nil
}
