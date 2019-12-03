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
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/api/isolate/isolateservice/v1"
	swarmingAPI "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/cmd/recorder/chromium/formats"
	"go.chromium.org/luci/resultdb/cmd/recorder/chromium/util"
	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/span"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	typepb "go.chromium.org/luci/resultdb/proto/type"
)

const (
	outputJSONFileName  = "output.json"
	swarmingAPIEndpoint = "_ah/api/swarming/v1/"
	maxInlineArtifact   = 1024 // store 1KIb+ artifacts in isolate
)

// DeriveProtosForWriting derives the protos with the data from the given task and request.
//
// The derived Invocation and TestResult protos will be written by the caller.
func DeriveProtosForWriting(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, req *pb.DeriveInvocationRequest) (*pb.Invocation, []*pb.TestResult, error) {
	if task.State == "PENDING" || task.State == "RUNNING" {
		// Tasks not yet completed should not be requested to be processed by the recorder.
		return nil, nil, errors.Reason(
			"unexpectedly incomplete state %q", task.State).Tag(grpcutil.FailedPreconditionTag).Err()
	}

	// Populate fields we will need in the base invocation.
	invID, err := GetInvocationID(ctx, task, req)
	if err != nil {
		return nil, nil, err
	}

	inv := &pb.Invocation{
		Name: invID.Name(),
	}

	// Populate timestamps if present.
	if inv.CreateTime, err = convertSwarmingTs(task.CreatedTs); err != nil {
		return nil, nil, errors.Annotate(err, "created_ts").Tag(grpcutil.FailedPreconditionTag).Err()
	}
	if inv.FinalizeTime, err = convertSwarmingTs(task.CompletedTs); err != nil {
		return nil, nil, errors.Annotate(err, "completed_ts").Tag(grpcutil.FailedPreconditionTag).Err()
	}

	// Decide how to continue based on task state.
	mustFetchOutputJSON := false
	switch task.State {
	// Tasks that got interrupted for which we expect no output just need to set the correct
	// Invocation state and are done.
	case "BOT_DIED", "CANCELED", "EXPIRED", "NO_RESOURCE", "KILLED":
		inv.State = pb.Invocation_INTERRUPTED
		return inv, nil, nil

	// Tasks that got interrupted for which we may get output need to set the correct Invocation state
	// but further processing may be needed.
	case "TIMED_OUT":
		inv.State = pb.Invocation_INTERRUPTED

	// For COMPLETED state, we expect normal completion and output.
	case "COMPLETED":
		inv.State = pb.Invocation_COMPLETED
		mustFetchOutputJSON = true

	default:
		return nil, nil, errors.Reason(
			"unknown swarming state %q", task.State).Tag(grpcutil.FailedPreconditionTag).Err()
	}

	// Parse swarming tags.
	baseVariant, gnTarget := parseSwarmingTags(task)
	testPathPrefix := ""
	if gnTarget != "" {
		testPathPrefix = fmt.Sprintf("gn:%s/", gnTarget)
	}

	// Fetch outputs, converting if any.
	var results []*pb.TestResult
	switch {
	case task.OutputsRef != nil && task.OutputsRef.Isolated != "":
		// If we have output, try to process it regardless.
		if results, err = processOutputs(ctx, task.OutputsRef, testPathPrefix, inv, req); err != nil {
			return nil, nil, err
		}

	case mustFetchOutputJSON:
		// Otherwise we expect output but have none, so fail.
		return nil, nil, errors.Reason(
			"missing expected isolated outputs").Tag(grpcutil.FailedPreconditionTag).Err()
	}

	// Apply the base variant.
	for _, r := range results {
		if len(r.Variant.GetDef()) == 0 {
			r.Variant = baseVariant
			continue
		}

		// Otherwise combine.
		for k, v := range baseVariant.Def {
			if _, ok := r.Variant.Def[k]; !ok {
				r.Variant.Def[k] = v
			}
		}
	}

	return inv, results, nil
}

func parseSwarmingTags(task *swarmingAPI.SwarmingRpcsTaskResult) (baseVariant *typepb.Variant, gnTarget string) {
	baseVariant = &typepb.Variant{Def: make(map[string]string, 3)}
	for _, t := range task.Tags {
		switch k, v := strpair.Parse(t); k {
		case "bucket":
			baseVariant.Def["bucket"] = v
		case "buildername":
			baseVariant.Def["builder"] = v
		case "test_suite":
			baseVariant.Def["test_suite"] = v
		case "gn_target":
			gnTarget = v
		}
	}
	return
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
func GetInvocationID(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, req *pb.DeriveInvocationRequest) (span.InvocationID, error) {
	// Get request information to include in ID.
	canonicalReq, _ := proto.Clone(req).(*pb.DeriveInvocationRequest)
	canonicalReq.SwarmingTask.Id = task.RunId

	h := sha256.New()
	if err := json.NewEncoder(h).Encode(canonicalReq); err != nil {
		return "", errors.Annotate(err, "").Tag(grpcutil.InternalTag).Err()
	}
	return span.InvocationID("swarming_" + hex.EncodeToString(h.Sum(nil))), nil
}

type isolatedClient struct {
	*isolatedclient.Client
	host      string
	namespace string
}

// processOutputs fetches the output.json from the given task and processes it using whichever
// additional isolated outputs necessary.
func processOutputs(ctx context.Context, outputsRef *swarmingAPI.SwarmingRpcsFilesRef, testPathPrefix string, inv *pb.Invocation, req *pb.DeriveInvocationRequest) (results []*pb.TestResult, err error) {
	isoClient := isolatedClient{
		Client:    isolatedclient.New(nil, internal.HTTPClient(ctx), outputsRef.Isolatedserver, outputsRef.Namespace, nil, nil),
		host:      strings.TrimPrefix(strings.TrimPrefix(outputsRef.Isolatedserver, "https://"), "http://"),
		namespace: outputsRef.Namespace,
	}

	// Get the isolated outputs.
	outputs, err := GetOutputs(ctx, isoClient.Client, outputsRef)
	if err != nil {
		return nil, err
	}

	// Fetch the output.json file itself and convert it.
	outputJSON, err := FetchOutputJSON(ctx, isoClient.Client, outputs)
	if err != nil {
		return nil, err
	}

	// Convert the isolated.File outputs to pb.Artifacts for later processing.
	outputsToProcess := make(map[string]*pb.Artifact, len(outputs))
	for path, f := range outputs {
		art := util.IsolatedFileToArtifact(isoClient.host, outputsRef.Namespace, path, &f)
		if art == nil {
			logging.Warningf(ctx, "Could not process artifact %q", path)
		}

		outputsToProcess[path] = art
	}

	// Convert the output JSON.
	if results, err = ConvertOutputJSON(ctx, inv, testPathPrefix, outputJSON, outputsToProcess); err != nil {
		return nil, err
	}
	if len(outputsToProcess) > 0 {
		logging.Warningf(ctx, "Unprocessed files:\n%s", util.IsolatedFilesToString(outputsToProcess))
	}

	// If some artifacts are large, store them in isolate.
	if err := isolateArtifacts(ctx, isoClient, results); err != nil {
		return nil, err
	}

	return results, nil
}

// GetOutputs gets the map of isolated.Files associated with the given task.
func GetOutputs(ctx context.Context, isoClient *isolatedclient.Client, ref *swarmingAPI.SwarmingRpcsFilesRef) (map[string]isolated.File, error) {
	// Fetch the isolate.
	buf := &bytes.Buffer{}
	if err := isoClient.Fetch(ctx, isolated.HexDigest(ref.Isolated), buf); err != nil {
		// TODO(jchinlee): handle error codes from here. Nonexisting isolates should not result in
		// INTERNAL or UNKNOWN.
		return nil, err
	}

	// Get the files.
	isolates := &isolated.Isolated{}
	if err := json.Unmarshal(buf.Bytes(), isolates); err != nil {
		return nil, errors.Annotate(err, "").Tag(grpcutil.InternalTag).Err()
	}
	return isolates.Files, nil
}

// FetchOutputJSON fetches the output.json given the outputs map, updating it in-place to mark the
// file as processed.
func FetchOutputJSON(ctx context.Context, isoClient *isolatedclient.Client, outputsToProcess map[string]isolated.File) ([]byte, error) {
	outputFile, ok := outputsToProcess[outputJSONFileName]
	if !ok {
		return nil, errors.Reason(
			"missing expected output %s in isolated outputs",
			outputJSONFileName).Tag(grpcutil.FailedPreconditionTag).Err()
	}

	// Now fetch (from the same server and namespace) the output file by digest.
	buf := &bytes.Buffer{}
	if err := isoClient.Fetch(ctx, outputFile.Digest, buf); err != nil {
		return nil, errors.Annotate(err,
			"%s digest %q", outputJSONFileName, outputFile.Digest).Err()
	}

	delete(outputsToProcess, outputJSONFileName)
	return buf.Bytes(), nil
}

// ConvertOutputJSON updates in-place the Invocation with the given data and extracts TestResults.
//
// It tries to convert to JSON Test Results format, then GTest format.
func ConvertOutputJSON(ctx context.Context, inv *pb.Invocation, testPathPrefix string, data []byte, outputsToProcess map[string]*pb.Artifact) ([]*pb.TestResult, error) {
	// Try to convert the buffer treating its format as the JSON Test Results Format.
	jsonFormat := &formats.JSONTestResults{}
	jsonErr := jsonFormat.ConvertFromJSON(ctx, bytes.NewReader(data))
	if jsonErr == nil {
		results, err := jsonFormat.ToProtos(ctx, testPathPrefix, inv, outputsToProcess)
		if err != nil {
			return nil, errors.Annotate(err, "converting as JSON Test Results Format").Err()
		}
		return results, nil
	}

	// Try to convert the buffer treating its format as that of GTests.
	gtestFormat := &formats.GTestResults{}
	gtestErr := gtestFormat.ConvertFromJSON(ctx, bytes.NewReader(data))
	if gtestErr == nil {
		results, err := gtestFormat.ToProtos(ctx, testPathPrefix, inv)
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
	if ts == "" {
		return nil, nil
	}

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

// isolateArtifacts isolates contents of large artifacts and replaces them with
// a link to isolate.
func isolateArtifacts(ctx context.Context, isoClient isolatedClient, results []*pb.TestResult) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, r := range results {
		for _, art := range r.OutputArtifacts {
			if len(art.Contents) > maxInlineArtifact {
				art := art
				eg.Go(func() error {
					return isolateArtifact(ctx, isoClient, art)
				})
			}
		}
	}

	return eg.Wait()
}

func isolateArtifact(ctx context.Context, isoClient isolatedClient, art *pb.Artifact) error {
	hash := isolated.GetHash(isoClient.namespace)
	digest := string(isolated.HashBytes(hash, art.Contents))

	states, err := isoClient.Contains(ctx, []*isolateservice.HandlersEndpointsV1Digest{
		{
			Digest: digest,
			Size:   int64(len(art.Contents)),
		},
	})
	if err != nil {
		return err
	}
	pushState := states[0]
	// Contains returns a nil state for items that do not exist.
	if pushState != nil {
		if err := isoClient.Push(ctx, pushState, isolatedclient.NewBytesSource(art.Contents)); err != nil {
			return errors.Annotate(err, "failed to isolate the artifact").Err()
		}
	}

	// Make art just a pointer.
	art.Contents = nil
	art.FetchUrl = util.IsolateFetchURL(isoClient.host, isoClient.namespace, digest)
	art.ViewUrl = util.IsolateViewURL(isoClient.host, isoClient.namespace, digest)
	return nil
}
