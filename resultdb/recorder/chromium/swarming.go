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
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
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
	// Get the swarming service to use.
	swarmingURL := "https://" + req.SwarmingTask.Hostname
	swarmSvc, err := getSwarmSvc(internal.HTTPClient(ctx), swarmingURL)
	if err != nil {
		return nil, err
	}

	// Get the swarming task, deduping if necessary.
	taskID := req.SwarmingTask.Id
	task, err := getSwarmingTask(ctx, taskID, swarmSvc)
	if err != nil {
		return nil, err
	}
	task, err = getOriginTask(ctx, task, swarmSvc)
	if err != nil {
		return nil, err
	}

	// Check if we even need to write this invocation: is it finalized?
	// TODO(jchinlee): Get Invocation from Spanner.
	var inv *pb.Invocation
	if inv != nil {
		if !pbutil.IsFinalized(inv.State) {
			return nil, errors.Reason(
				"attempting to derive an existing non-finalized invocation").Err()
		}

		return inv, nil
	}

	inv, _, err = deriveProtosForWriting(ctx, task, req)
	if err != nil {
		return nil, errors.Annotate(err, "task %s on %s", taskID, swarmingURL).Err()
	}

	// TODO(jchinlee): Write Invocation and TestResults to Spanner.

	return inv, err
}

// deriveProtosForWriting derives the protos with the data from the given task and request.
//
// The derived Invocation and TestResult protos will be written by the caller.
func deriveProtosForWriting(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, req *pb.DeriveInvocationRequest) (*pb.Invocation, []*pb.TestResult, error) {
	// Populate fields we will need in the base invocation.
	invID, err := getInvocationID(ctx, task, req)
	if err != nil {
		return nil, nil, err
	}

	createTime, err := convertSwarmingTs(task.CreatedTs)
	if err != nil {
		return nil, nil, errors.Annotate(err, "created_ts").Err()
	}

	finalizeTime, err := convertSwarmingTs(task.CompletedTs)
	if err != nil {
		return nil, nil, errors.Annotate(err, "completed_ts").Err()
	}

	inv := &pb.Invocation{
		Name:               "invocations/" + invID,
		CreateTime:         createTime,
		FinalizeTime:       finalizeTime,
		BaseTestVariantDef: req.BaseTestVariant,
	}

	// Get the outputs ref.
	outputsRef := task.OutputsRef
	outputsIsolatedHash := ""
	if outputsRef != nil {
		outputsIsolatedHash = outputsRef.Isolated
	}

	// Decide how to continue based on task state; fetch output JSON if needed.
	var outputJSON []byte
	switch task.State {
	// Tasks not yet completed should not be requested to be processed by the recorder.
	case "PENDING", "RUNNING":
		err = errors.Reason("unexpectedly incomplete").Err()

	// Tasks that got interrupted for which we expect no output just need to set the correct
	// Invocation state.
	case "BOT_DIED", "CANCELED", "EXPIRED", "NO_RESOURCE":
		inv.State = pb.Invocation_INTERRUPTED

	// Tasks that got interrupted for which we may get output need to set the correct Invocation state
	// and get outputs if there happen to be any.
	case "KILLED", "TIMED_OUT":
		inv.State = pb.Invocation_INTERRUPTED
		if outputsIsolatedHash == "" {
			break
		}
		outputJSON, err = fetchOutputJSON(ctx, task, outputsRef)

	// For COMPLETED state, we expect normal completion and output.
	case "COMPLETED":
		if outputsIsolatedHash == "" {
			err = errors.Reason("missing expected isolated outputs").Err()
			break
		}
		outputJSON, err = fetchOutputJSON(ctx, task, outputsRef)

	default:
		err = errors.Reason("unknown swarming state").Err()
	}

	if err != nil {
		return nil, nil, errors.Annotate(err, "processing task state %s", task.State).Err()
	}

	// Get invocation and test results using the correct format if we have outputs.
	var results []*pb.TestResult
	if outputJSON != nil {
		results, err = convertOutputJSON(ctx, inv, req, outputJSON)
		if err != nil {
			return nil, nil, err
		}
	}

	return inv, results, nil
}

// getSwarmSvc gets a swarming service for the given URL.
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

// getOriginTask gets the swarming task of which the given task is a dupe, or itself if it isn't.
func getOriginTask(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, swarmSvc *swarmingAPI.Service) (*swarmingAPI.SwarmingRpcsTaskResult, error) {
	// If the task was deduped, then the invocation associated with it is just the one associated
	// to the task from which it was deduped.
	var err error
	for task.DedupedFrom != "" {
		task, err = getSwarmingTask(ctx, task.DedupedFrom, swarmSvc)
		if err != nil {
			return nil, err
		}
	}

	return task, nil
}

// getInvocationID gets the ID of the invocation associated with a task and swarming service.
func getInvocationID(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, req *pb.DeriveInvocationRequest) (string, error) {
	// Get request information to include in ID.
	canonicalReq, _ := proto.Clone(req).(*pb.DeriveInvocationRequest)
	canonicalReq.SwarmingTask.Id = task.RunId

	h := sha256.New()
	if err := json.NewEncoder(h).Encode(canonicalReq); err != nil {
		return "", err
	}
	return "from_swarming_" + hex.EncodeToString(h.Sum(nil)), nil
}

// fetchOutputJSON fetches the output.json from the given task with the given ref.
func fetchOutputJSON(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, ref *swarmingAPI.SwarmingRpcsFilesRef) ([]byte, error) {
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

// convertOutputJSON updates in-place the Invocation with the given data and extracts TestResults.
//
// It tries to convert to JSON Test Results format, then GTest format.
func convertOutputJSON(ctx context.Context, inv *pb.Invocation, req *pb.DeriveInvocationRequest, data []byte) ([]*pb.TestResult, error) {
	if req == nil || req.BaseTestVariant == nil {
		return nil, errors.Reason("missing required req.BaseTestVariant").Err()
	}

	// Validate BaseTestVariant passed in the request.
	if err := resultdb.VariantDefMap(req.BaseTestVariant.Def).Validate(); err != nil {
		return nil, errors.Annotate(
			err, "invalid base test variant %q", req.BaseTestVariant.Def).
			Tag(grpcutil.InvalidArgumentTag).Err()
	}

	// Try to convert the buffer treating its format as the JSON Test Results Format.
	jsonFormat := &formats.JSONTestResults{}
	jsonErr := jsonFormat.ConvertFromJSON(ctx, bytes.NewReader(data))
	if jsonErr == nil {
		results, err := jsonFormat.ToProtos(ctx, req, inv)
		if err != nil {
			return nil, errors.Annotate(err, "converting as JSON Test Results Format").Err()
		}
		return results, nil
	}

	// Try to convert the buffer treating its format as that of GTests.
	gtestFormat := &formats.GTestResults{}
	gtestErr := gtestFormat.ConvertFromJSON(ctx, bytes.NewReader(data))
	if gtestErr == nil {
		results, err := gtestFormat.ToProtos(ctx, req, inv)
		if err != nil {
			return nil, errors.Annotate(err, "converting as GTest results format").Err()
		}
		return results, nil
	}

	return nil, errors.NewMultiError(jsonErr, gtestErr)
}

// convertSwarmingTs converts a swarming-formatted string to a tspb.Timestamp.
func convertSwarmingTs(ts string) (*tspb.Timestamp, error) {
	if ts == "" {
		return nil, errors.Reason("no timestamp to convert").Err()
	}

	// Timestamp strings from swarming should be RFC3339 without the trailing Z; check in case.
	if ts[len(ts)-1] != byte('Z') {
		ts += "Z"
	}

	var tpb *tspb.Timestamp
	var err error
	t, err := time.Parse(time.RFC3339, ts)
	if err == nil {
		tpb, err = ptypes.TimestampProto(t)
	}
	return tpb, errors.Annotate(err, "converting timestamp string %q", ts).Err()
}
