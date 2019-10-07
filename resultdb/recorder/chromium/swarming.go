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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"

	swarmingAPI "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb"
	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/recorder/chromium/formats"
)

const (
	outputJSONFileName  = "output.json"
	swarmingAPIEndpoint = "_ah/api/swarming/v1/"
)

// DeriveInvocation derives the invocation associated with the given swarming task.
//
// If the task is a dedup of another task, the invocation returned is the underlying one; otherwise,
// the invocation returned is associated with the swarming task itself.
func DeriveInvocation(ctx context.Context, req *pb.DeriveInvocationRequest) (*pb.Invocation, error) {
	swarmingURL := "https://" + req.SwarmingTask.Hostname
	taskID := req.SwarmingTask.Id
	cl := internal.HTTPClient(ctx)

	swarmSvc, err := getSwarmSvc(cl, swarmingURL)
	if err != nil {
		return nil, err
	}

	// Get the swarming task.
	task, err := getSwarmingTask(ctx, taskID, swarmSvc)
	if err != nil {
		return nil, err
	}
	if task.State == "PENDING" || task.State == "RUNNING" {
		return nil, errors.Reason(
			"cannot write Invocation of incomplete task %s on %s (%s)",
			taskID, swarmingURL, task.State).Err()
	}

	// Check if we even need to write this invocation: is it finalized?
	// TODO(jchinlee): Actually use returned Invocation ID.
	_, err = getInvocationID(ctx, task, swarmSvc, req)
	if err != nil {
		return nil, err
	}

	// TODO(jchinlee): Get Invocation from Spanner.
	var inv *pb.Invocation
	if inv != nil {
		if !pbutil.IsFinalized(inv.State) {
			return nil, errors.Reason(
				"attempting to derive an existing non-finalized invocation").Err()
		}

		return inv, nil
	}

	// We may need to write the invocation, so fetch the output JSON.
	outputJSON, err := fetchOutputJSON(ctx, task)
	if err != nil {
		return nil, errors.Annotate(err, "task %s (%s)", taskID, swarmingURL).Err()
	}

	// Get invocation and test results using the correct format.
	inv, _, err = convertOutputJSON(ctx, req, outputJSON)
	if err != nil {
		return nil, err
	}

	return inv, nil
}

func getSwarmSvc(cl *http.Client, swarmingURL string) (*swarmingAPI.Service, error) {
	swarmSvc, err := swarmingAPI.New(cl)
	if err != nil {
		return nil, err
	}

	swarmSvc.BasePath = fmt.Sprintf("%s/%s", swarmingURL, swarmingAPIEndpoint)
	return swarmSvc, nil
}

// getSwarmingTask fetches the task from swarming, annotating errors with gRPC codes as needed.
func getSwarmingTask(ctx context.Context, taskID string, swarmSvc *swarmingAPI.Service) (*swarmingAPI.SwarmingRpcsTaskResult, error) {
	task, err := swarmSvc.Task.Result(taskID).Context(ctx).Do()
	if err, ok := err.(*googleapi.Error); ok {
		switch {
		case err.Code == http.StatusUnauthorized:
			return nil, errors.Annotate(err, "").Tag(grpcutil.UnauthenticatedTag).Err()
		case err.Code == http.StatusForbidden:
			return nil, errors.Annotate(err, "").Tag(grpcutil.PermissionDeniedTag).Err()
		case err.Code == http.StatusNotFound:
			return nil, errors.Annotate(err, "swarming task not found").Tag(grpcutil.NotFoundTag).Err()
		case err.Code >= 500:
			return nil, errors.Annotate(err, "swarming unavailable").Tag(
				transient.Tag, grpcutil.InternalTag).Err()
		}
	}

	return task, errors.Annotate(err, "").Tag(grpcutil.InternalTag).Err()
}

// fetchOutputJSON fetches the output.json from the given task on the given host.
func fetchOutputJSON(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult) ([]byte, error) {
	// Get the ref for the isolated outs of the given task.
	ref, err := getOutputsRef(ctx, task)
	if err != nil {
		return nil, err
	}

	// Get isolated client for getting isolated objects.
	cl := internal.HTTPClient(ctx)
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
	// TODO(jchinlee): Handle BOT_DIED, which may have the below state but shouldn't error internally.
	if task.OutputsRef == nil || task.OutputsRef.Isolated == "" {
		return nil, errors.Reason("no isolated outputs").Err()
	}

	return task.OutputsRef, nil
}

// getInvocationID gets the ID of the invocation associated with a task and swarming service.
func getInvocationID(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, swarmSvc *swarmingAPI.Service, req *pb.DeriveInvocationRequest) (string, error) {
	// If the task was deduped, then the invocation associated with it is just the one associated
	// to the task from which it was deduped.
	var err error
	for task.DedupedFrom != "" {
		task, err = getSwarmingTask(ctx, task.DedupedFrom, swarmSvc)
		if err != nil {
			return "", err
		}
	}

	// Get request information to include in ID.
	canonicalReq, _ := proto.Clone(req).(*pb.DeriveInvocationRequest)
	canonicalReq.SwarmingTask.Id = task.RunId
	h := sha256.New()
	if err := json.NewEncoder(h).Encode(canonicalReq); err != nil {
		return "", err
	}
	return "from_swarming_" + hex.EncodeToString(h.Sum(nil)), nil
}

// convertOutputJSON converts the given data to Invocation and TestResults.
//
// It tries to convert to JSON Test Results format, then GTest format.
func convertOutputJSON(ctx context.Context, req *pb.DeriveInvocationRequest, data []byte) (*pb.Invocation, []*pb.TestResult, error) {
	// Validate BaseTestVariant passed in the request.
	if err := resultdb.VariantDefMap(req.BaseTestVariant.Def).Validate(); err != nil {
		return nil, nil, errors.Annotate(
			err, "invalid base test variant %q", req.BaseTestVariant.Def).
			Tag(grpcutil.InvalidArgumentTag).Err()
	}

	inv := &pb.Invocation{BaseTestVariantDef: req.BaseTestVariant}

	// Try to convert the buffer treating its format as the JSON Test Results Format.
	jsonFormat := &formats.JSONTestResults{}
	jsonErr := jsonFormat.ConvertFromJSON(ctx, bytes.NewReader(data))
	if jsonErr == nil {
		results, err := jsonFormat.ToProtos(ctx, req, inv)
		if err != nil {
			return nil, nil, errors.Annotate(err, "converting as JSON Test Results Format").Err()
		}
		return inv, results, nil
	}

	// Try to convert the buffer treating its format as that of GTests.
	gtestFormat := &formats.GTestResults{}
	gtestErr := gtestFormat.ConvertFromJSON(ctx, bytes.NewReader(data))
	if gtestErr == nil {
		results, err := gtestFormat.ToProtos(ctx, req, inv)
		if err != nil {
			return nil, nil, errors.Annotate(err, "converting as GTest results format").Err()
		}
		return inv, results, nil
	}

	return nil, nil, errors.NewMultiError(jsonErr, gtestErr)
}
