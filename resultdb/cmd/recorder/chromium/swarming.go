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
	"strings"
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

	"go.chromium.org/luci/resultdb/cmd/recorder/chromium/formats"
	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const (
	outputJSONFileName  = "output.json"
	swarmingAPIEndpoint = "_ah/api/swarming/v1/"
)

// DeriveProtosForWriting derives the protos with the data from the given task and request.
//
// The derived Invocation and TestResult protos will be written by the caller.
func DeriveProtosForWriting(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, req *pb.DeriveInvocationRequest) (*pb.Invocation, []*pb.TestResult, error) {
	// Populate fields we will need in the base invocation.
	invID, err := GetInvocationID(ctx, task, req)
	if err != nil {
		return nil, nil, err
	}

	inv := &pb.Invocation{
		Name:               pbutil.InvocationName(invID),
		BaseTestVariantDef: req.BaseTestVariant,
	}

	if inv.CreateTime, err = convertSwarmingTs(task.CreatedTs); err != nil {
		return nil, nil, errors.Annotate(err, "created_ts").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	if inv.FinalizeTime, err = convertSwarmingTs(task.CompletedTs); err != nil {
		return nil, nil, errors.Annotate(err, "completed_ts").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	// Decide how to continue based on task state.
	mustFetchOutputJSON := false
	switch task.State {
	// Tasks not yet completed should not be requested to be processed by the recorder.
	case "PENDING", "RUNNING":
		return nil, nil, errors.Reason(
			"unexpectedly incomplete state %q", task.State).Tag(grpcutil.FailedPreconditionTag).Err()

	// Tasks that got interrupted for which we expect no output just need to set the correct
	// Invocation state and are done.
	case "BOT_DIED", "CANCELED", "EXPIRED", "NO_RESOURCE":
		inv.State = pb.Invocation_INTERRUPTED
		return inv, nil, nil

	// Tasks that got interrupted for which we may get output need to set the correct Invocation state
	// but further processing may be needed.
	case "KILLED", "TIMED_OUT":
		inv.State = pb.Invocation_INTERRUPTED

	// For COMPLETED state, we expect normal completion and output.
	case "COMPLETED":
		inv.State = pb.Invocation_COMPLETED
		mustFetchOutputJSON = true

	default:
		return nil, nil, errors.Reason(
			"unknown swarming state %q", task.State).Tag(grpcutil.InvalidArgumentTag).Err()
	}

	// Fetch outputs, converting if any.
	var results []*pb.TestResult
	switch {
	case task.OutputsRef != nil && task.OutputsRef.Isolated != "":
		// If we have output, fetch it regardless.
		outputJSON, err := FetchOutputJSON(ctx, task, task.OutputsRef)
		if err != nil {
			return nil, nil, err
		}
		if results, err = ConvertOutputJSON(ctx, inv, req, outputJSON); err != nil {
			return nil, nil, err
		}

	case mustFetchOutputJSON:
		// Otherwise we expect output but have none, so fail.
		return nil, nil, errors.Reason(
			"missing expected isolated outputs").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	return inv, results, nil
}

// GetSwarmSvc gets a swarming service for the given URL.
func GetSwarmSvc(cl *http.Client, swarmingURL string) (*swarmingAPI.Service, error) {
	swarmSvc, err := swarmingAPI.New(cl)
	if err != nil {
		return nil, err
	}

	swarmSvc.BasePath = fmt.Sprintf("%s/%s", swarmingURL, swarmingAPIEndpoint)
	return swarmSvc, nil
}

// GetSwarmingTask fetches the task from swarming, annotating errors with gRPC codes as needed.
func GetSwarmingTask(ctx context.Context, taskID string, swarmSvc *swarmingAPI.Service) (*swarmingAPI.SwarmingRpcsTaskResult, error) {
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

// GetOriginTask gets the swarming task of which the given task is a dupe, or itself if it isn't.
func GetOriginTask(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, swarmSvc *swarmingAPI.Service) (*swarmingAPI.SwarmingRpcsTaskResult, error) {
	// If the task was deduped, then the invocation associated with it is just the one associated
	// to the task from which it was deduped.
	for task.DedupedFrom != "" {
		var err error
		if task, err = GetSwarmingTask(ctx, task.DedupedFrom, swarmSvc); err != nil {
			return nil, err
		}
	}

	return task, nil
}

// GetInvocationID gets the ID of the invocation associated with a task and swarming service.
func GetInvocationID(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, req *pb.DeriveInvocationRequest) (string, error) {
	// Get request information to include in ID.
	canonicalReq, _ := proto.Clone(req).(*pb.DeriveInvocationRequest)
	canonicalReq.SwarmingTask.Id = task.RunId

	h := sha256.New()
	if err := json.NewEncoder(h).Encode(canonicalReq); err != nil {
		return "", errors.Annotate(err, "").Tag(grpcutil.InternalTag).Err()
	}
	return "swarming_" + hex.EncodeToString(h.Sum(nil)), nil
}

// FetchOutputJSON fetches the output.json from the given task with the given ref.
func FetchOutputJSON(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, ref *swarmingAPI.SwarmingRpcsFilesRef) ([]byte, error) {
	// Get isolated client for getting isolated objects.
	cl := internal.HTTPClient(ctx)
	isoClient := isolatedclient.New(nil, cl, ref.Isolatedserver, ref.Namespace, nil, nil)

	// Fetch the isolate.
	logging.Infof(
		ctx, "Fetching %s for isolated outs of task %s", ref.Isolated, task.TaskId)
	buf := &bytes.Buffer{}
	if err := isoClient.Fetch(ctx, isolated.HexDigest(ref.Isolated), buf); err != nil {
		// TODO(jchinlee): handle error codes from here. Nonexisting isolates should not result in
		// INTERNAL or UNKNOWN.
		return nil, err
	}

	isolates := &isolated.Isolated{}
	if err := json.Unmarshal(buf.Bytes(), isolates); err != nil {
		return nil, errors.Annotate(err, "").Tag(grpcutil.InternalTag).Err()
	}

	outputFile, ok := isolates.Files[outputJSONFileName]
	if !ok {
		return nil, errors.Reason(
			"missing expected output %s in isolated outputs",
			outputJSONFileName).Tag(grpcutil.FailedPreconditionTag).Err()
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

// ConvertOutputJSON updates in-place the Invocation with the given data and extracts TestResults.
//
// It tries to convert to JSON Test Results format, then GTest format.
func ConvertOutputJSON(ctx context.Context, inv *pb.Invocation, req *pb.DeriveInvocationRequest, data []byte) ([]*pb.TestResult, error) {
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

	// Conversion with either format failed, but this code is meant specifically for them.
	return nil, errors.Annotate(
		errors.NewMultiError(jsonErr, gtestErr), "").Tag(grpcutil.InternalTag).Err()
}

// convertSwarmingTs converts a swarming-formatted string to a tspb.Timestamp.
func convertSwarmingTs(ts string) (*tspb.Timestamp, error) {
	// Timestamp strings from swarming should be RFC3339 without the trailing Z; check in case.
	if !strings.HasSuffix(ts, "Z") {
		ts += "Z"
	}

	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return nil, errors.Annotate(err, "converting timestamp string %q", ts).Err()
	}
	return ptypes.TimestampProto(t)
}
