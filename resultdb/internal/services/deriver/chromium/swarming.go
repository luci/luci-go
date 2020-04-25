// Copyright 2020 The LUCI Authors.
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
	"html/template"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	swarmingAPI "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/services/deriver/chromium/formats"
	"go.chromium.org/luci/resultdb/internal/services/deriver/chromium/util"
	"go.chromium.org/luci/resultdb/internal/span"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
	typepb "go.chromium.org/luci/resultdb/proto/type"
)

const (
	swarmingAPIEndpoint = "_ah/api/swarming/v1/"
)

var (
	// Look for the output JSON trying the below possibilities in the given order.
	outputJSONFileNames = []string{"output.json", "full_results.json"}

	// SummaryHtml template for the results that are converted from a swarming task.
	summaryTmpl = template.Must(template.New("task summary").Parse(`
		<a href="{{ .url }}">task</a> is {{.state}}
	`))
)

// DeriveChromiumInvocation derives the Invocation proto from the given task and request.
//
// The derived Invocation will be written by the caller.
func DeriveChromiumInvocation(task *swarmingAPI.SwarmingRpcsTaskResult, req *pb.DeriveChromiumInvocationRequest) (*pb.Invocation, error) {
	if task.State == "PENDING" || task.State == "RUNNING" {
		// Tasks not yet completed should not be requested to be processed by the deriver.
		s := status.Newf(codes.FailedPrecondition, "task %s is not complete yet", task.TaskId)
		s = appstatus.MustWithDetails(s, &errdetails.PreconditionFailure{
			Violations: []*errdetails.PreconditionFailure_Violation{{
				Type: pb.DeriveChromiumInvocationPreconditionFailureType_INCOMPLETE_CHROMIUM_SWARMING_TASK.String(),
			}},
		})
		return nil, appstatus.ToError(s)
	}

	// Populate fields we will need in the base invocation.
	inv := &pb.Invocation{
		Name:             GetInvocationID(task, req).Name(),
		State:            pb.Invocation_FINALIZED,
		ProducerResource: fmt.Sprintf("//%s/tasks/%s", req.SwarmingTask.Hostname, req.SwarmingTask.Id),
	}
	var err error
	if inv.CreateTime, err = convertSwarmingTs(task.CreatedTs); err != nil {
		return nil, invalidTaskf("invalid task creation time %q: %s", task.CreatedTs, err)
	}
	if inv.FinalizeTime, err = convertSwarmingTs(task.CompletedTs); err != nil {
		return nil, invalidTaskf("invalid task completion time %q: %s", task.CreatedTs, err)
	}
	inv.Deadline = inv.FinalizeTime

	// Decide how to continue based on task state.
	switch task.State {
	// Tasks that got interrupted for which we expect no output just need to set the correct
	// Invocation state and are done.
	case "BOT_DIED", "CANCELED", "EXPIRED", "NO_RESOURCE", "KILLED", "TIMED_OUT":
		inv.Interrupted = true
		return inv, nil
	// For COMPLETED state, we expect normal completion.
	case "COMPLETED":
		return inv, nil
	default:
		return nil, invalidTaskf("unknown state %q", task.State)
	}
}

// TestResult combines test result with the associated artifacts.
type TestResult struct {
	*pb.TestResult
	Artifacts []*pb.Artifact
}

// DeriveTestResults derives the protos with the data from the given task and request.
//
// The derived Invocation and TestResult protos will be written by the caller.
func DeriveTestResults(ctx context.Context, task *swarmingAPI.SwarmingRpcsTaskResult, req *pb.DeriveChromiumInvocationRequest, inv *pb.Invocation) ([]*TestResult, error) {
	// Parse swarming tags.
	baseVariant, ninjaTarget := parseSwarmingTags(task)
	testIDPrefix := ""
	taskTestID := ""
	if ninjaTarget != "" {
		taskTestID = fmt.Sprintf("ninja:%s", ninjaTarget)
		testIDPrefix = fmt.Sprintf("%s/", taskTestID)
	}

	// Fetch outputs, converting if any.
	var results []*TestResult
	var err error
	ref := task.OutputsRef

	if ref != nil && ref.Isolated != "" {
		if results, err = processOutputs(ctx, ref, testIDPrefix, inv, req); err != nil {
			logging.Warningf(ctx, "isolated outputs at %q in %q, %q: %s",
				ref.Isolated, ref.Isolatedserver, ref.Namespace, err)
		}
	} else {
		err = fmt.Errorf("no output")
	}

	if err != nil {
		// Either no output to process or we don't understand the output,
		// fall back to convert the whole task as one result.
		result, err := convertTaskToResult(taskTestID, task, req)
		if err != nil {
			return nil, attachInvalidTaskf(err, "failed to convert the task to a test result")
		}
		results = []*TestResult{{TestResult: result}}
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

	return results, nil
}

func parseSwarmingTags(task *swarmingAPI.SwarmingRpcsTaskResult) (baseVariant *typepb.Variant, ninjaTarget string) {
	baseVariant = &typepb.Variant{Def: make(map[string]string, 3)}
	for _, t := range task.Tags {
		switch k, v := strpair.Parse(t); k {
		case "bucket":
			baseVariant.Def["bucket"] = v
		case "buildername":
			baseVariant.Def["builder"] = v
		case "test_suite":
			baseVariant.Def["test_suite"] = v
		case "ninja_target":
			ninjaTarget = v
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
		case err.Code == http.StatusNotFound:
			return nil, appstatus.Attachf(err, codes.NotFound, "swarming task %s not found", taskID)

		case err.Code >= 500:
			return nil, appstatus.Attachf(err, codes.Internal, "transient swarming error")
		}
	}

	return task, err
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
func GetInvocationID(task *swarmingAPI.SwarmingRpcsTaskResult, req *pb.DeriveChromiumInvocationRequest) span.InvocationID {
	return span.InvocationID(fmt.Sprintf("task:%s:%s", req.SwarmingTask.Hostname, task.RunId))
}

func getTaskResultStatus(task *swarmingAPI.SwarmingRpcsTaskResult) pb.TestStatus {
	switch {
	case task.State == "TIMED_OUT":
		return pb.TestStatus_ABORT
	case task.Failure:
		return pb.TestStatus_FAIL
	default:
		return pb.TestStatus_PASS
	}
}

// convertTaskToResult uses a swarming task's information to create a test result.
// It's the fallback when resultdb fails to parse the results using json and
// gtest result formats.
func convertTaskToResult(testID string, task *swarmingAPI.SwarmingRpcsTaskResult, req *pb.DeriveChromiumInvocationRequest) (*pb.TestResult, error) {
	resultStatus := getTaskResultStatus(task)
	ret := &pb.TestResult{
		// Use ninja target as test_id.
		TestId:   testID,
		Expected: resultStatus == pb.TestStatus_PASS,
		Status:   resultStatus,
	}

	// Add the swarming task's url and state to summaryHTML.
	buf := &strings.Builder{}
	err := summaryTmpl.Execute(buf, map[string]interface{}{
		"url":   fmt.Sprintf("https://%s/%s", req.SwarmingTask.Hostname, req.SwarmingTask.Id),
		"state": task.State,
	})
	if err != nil {
		return nil, err
	}
	ret.SummaryHtml = buf.String()

	return ret, nil
}

// processOutputs fetches the output.json from the given task and processes it
// using whichever artifacts necessary.
func processOutputs(ctx context.Context, outputsRef *swarmingAPI.SwarmingRpcsFilesRef, testIDPrefix string, inv *pb.Invocation, req *pb.DeriveChromiumInvocationRequest) ([]*TestResult, error) {

	isoClient := isolatedclient.NewClient(
		outputsRef.Isolatedserver, isolatedclient.WithAuthClient(internal.HTTPClient(ctx)), isolatedclient.WithNamespace(outputsRef.Namespace))

	// Get the isolated outputs.
	outputs, err := GetOutputs(ctx, isoClient, outputsRef)
	if err != nil {
		return nil, errors.Annotate(err, "getting isolated outputs").Err()
	}
	availableArtifacts := stringset.New(len(outputs))
	for path := range outputs {
		availableArtifacts.Add(util.NormalizeIsolatedPath(path))
	}

	// Fetch the output.json file itself and convert it.
	outputJSON, err := FetchOutputJSON(ctx, isoClient, outputs)
	if err != nil {
		return nil, errors.Annotate(err, "getting output JSON file").Err()
	}

	// Convert the output JSON.
	results, err := convertOutputJSON(ctx, inv, testIDPrefix, outputJSON, availableArtifacts)
	if err != nil {
		return nil, attachInvalidTaskf(err, "invalid output: %s", err)
	}

	// Convert formats.TestResult to TestResult and convert the isolated.File
	// to pb.Artifacts.
	ret := make([]*TestResult, len(results))
	for i, r := range results {
		ret[i] = &TestResult{
			TestResult: r.TestResult,
			Artifacts:  make([]*pb.Artifact, 0, len(r.Artifacts)),
		}

		for name, relPath := range r.Artifacts {
			a := isolatedFileToArtifact(outputsRef.Isolatedserver, outputsRef.Namespace, relPath, outputs[relPath], name)
			if a != nil {
				ret[i].Artifacts = append(ret[i].Artifacts, a)
			}
		}
	}
	return ret, nil
}

func isolatedFileToArtifact(isolateServer, ns, relPath string, f isolated.File, artifactID string) *pb.Artifact {
	// We don't know how to handle symlink files, so return nil for the caller to deal with it.
	if f.Link != nil {
		return nil
	}

	a := &pb.Artifact{
		ArtifactId: artifactID,
		FetchUrl:   internal.IsolateURL(util.IsolateServerToHost(isolateServer), ns, string(f.Digest)),
	}

	if f.Size != nil {
		a.SizeBytes = *f.Size
	}

	switch path.Ext(relPath) {
	case ".txt":
		a.ContentType = "text/plain"
	case ".html":
		a.ContentType = "text/html"
	case ".png":
		a.ContentType = "image/png"
	}

	return a
}

// GetOutputs gets the map of isolated.Files associated with the given task.
func GetOutputs(ctx context.Context, isoClient *isolatedclient.Client, ref *swarmingAPI.SwarmingRpcsFilesRef) (map[string]isolated.File, error) {
	// Fetch the isolate.
	buf := &bytes.Buffer{}
	if err := isoClient.Fetch(ctx, isolated.HexDigest(ref.Isolated), buf); err != nil {
		if status, ok := lhttp.IsHTTPError(err); ok {
			switch {
			case status == http.StatusNotFound:
				return nil, appstatus.Attachf(err, codes.NotFound, "swarming task's output %s is not found", ref.Isolated)
			case status >= 500:
				return nil, transient.Tag.Apply(appstatus.Attachf(err, codes.Internal, "transient swarming error"))
			}
		}
		return nil, err
	}

	// Get the files.
	isolates := &isolated.Isolated{}
	if err := json.Unmarshal(buf.Bytes(), isolates); err != nil {
		return nil, err
	}
	return isolates.Files, nil
}

// FetchOutputJSON fetches the output.json given the outputs map, updating it in-place to mark the
// file as processed.
func FetchOutputJSON(ctx context.Context, isoClient *isolatedclient.Client, outputs map[string]isolated.File) ([]byte, error) {
	// Check the different possibilities for the output JSON name.
	var outputJSONFileName string
	var outputFile *isolated.File
	for _, outputJSONFileName = range outputJSONFileNames {
		if file, ok := outputs[outputJSONFileName]; ok {
			outputFile = &file
			break
		}
	}

	if outputFile == nil {
		return nil, invalidTaskf("missing expected output in isolated outputs, tried {%s}", strings.Join(outputJSONFileNames, ", "))
	}

	if *outputFile.Size == 0 {
		return nil, invalidTaskf("empty output file %s", outputJSONFileName)
	}

	// Now fetch (from the same server and namespace) the output file by digest.
	buf := &bytes.Buffer{}
	if err := isoClient.Fetch(ctx, outputFile.Digest, buf); err != nil {
		return nil, errors.Annotate(err,
			"%s digest %q", outputJSONFileName, outputFile.Digest).Err()
	}

	return buf.Bytes(), nil
}

// convertOutputJSON updates in-place the Invocation with the given data and extracts TestResults.
//
// It tries to convert to JSON Test Results format, then GTest format.
func convertOutputJSON(ctx context.Context, inv *pb.Invocation, testIDPrefix string, data []byte, availableArtifacts stringset.Set) ([]*formats.TestResult, error) {
	// Try to convert the buffer treating its format as the JSON Test Results Format.
	jsonFormat := &formats.JSONTestResults{}
	jsonErr := jsonFormat.ConvertFromJSON(ctx, bytes.NewReader(data))
	if jsonErr == nil {
		results, err := jsonFormat.ToProtos(ctx, testIDPrefix, inv, availableArtifacts)
		if err != nil {
			return nil, errors.Annotate(err, "converting as JSON Test Results Format").Err()
		}
		return results, nil
	}
	logging.Warningf(ctx, "did not recognize as JSON Test Results: %s", jsonErr)

	// Try to convert the buffer treating its format as that of GTests.
	gtestFormat := &formats.GTestResults{}
	gtestErr := gtestFormat.ConvertFromJSON(ctx, bytes.NewReader(data))
	if gtestErr == nil {
		results, err := gtestFormat.ToProtos(ctx, testIDPrefix, inv)
		if err != nil {
			return nil, errors.Annotate(err, "converting as GTest results format").Err()
		}
		return results, nil
	}
	logging.Warningf(ctx, "did not recognize as GTest: %s", gtestErr)

	// Conversion with either format failed, but we don't support other formats.
	return nil, errors.NewMultiError(gtestErr, jsonErr)
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

func invalidTaskf(format string, args ...interface{}) error {
	return appstatus.Errorf(codes.FailedPrecondition, "invalid task: "+format, args...)
}

func attachInvalidTaskf(err error, format string, args ...interface{}) error {
	return appstatus.Attachf(err, codes.FailedPrecondition, "invalid task: "+format, args...)
}
