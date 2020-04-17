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

package formats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/text/indented"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/resultdb/internal/services/deriver/chromium/util"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

const testNamePrefixKey = "test_name_prefix"

var (
	// Subdirectory of isolated outputs root identifying run index.
	testRunSubdirRe = regexp.MustCompile("retry_([0-9]+)/")

	// Regexp for matching about:blank reference artifact.
	aboutBlankRe = regexp.MustCompile(`\babout:blank$`)

	// Known subdirectories of the isolated outputs root where artifacts might be located.
	artifactDirectories = []string{
		"artifacts",
		"layout-test-results",
	}
)

// JSONTestResults represents the structure in
// https://chromium.googlesource.com/chromium/src/+/master/docs/testing/json_test_results_format.md
//
// Deprecated fields and fields not used by Test Results are omitted.
type JSONTestResults struct {
	Interrupted bool `json:"interrupted"`

	PathDelimiter string `json:"path_delimiter"`

	TestsRaw json.RawMessage `json:"tests"`
	Tests    map[string]*TestFields

	Version int32 `json:"version"`

	ArtifactTypes map[string]string `json:"artifact_types"`

	BuildNumber string `json:"build_number"`
	BuilderName string `json:"builder_name"`

	// Metadata associated with results, which may include a list of expectation_files, or
	// test_name_prefix e.g. in GPU tests (distinct from test_id_prefix passed in the recorder API
	// request).
	Metadata map[string]json.RawMessage `json:"metadata"`
}

// TestFields represents the test fields structure in
// https://chromium.googlesource.com/chromium/src/+/master/docs/testing/json_test_results_format.md
//
// Deprecated fields and fields not used by Test Results are omitted.
type TestFields struct {
	Actual   string `json:"actual"`
	Expected string `json:"expected"`

	// TODO(crbug/1034021): Support only map[string][]string, as spec'd by the JSON Test Results
	// Format.
	ArtifactsRaw map[string]json.RawMessage `json:"artifacts"`
	Artifacts    map[string][]string

	Time  float64   `json:"time"`
	Times []float64 `json:"times"`
}

// ConvertFromJSON converts a JSON of test results in the JSON Test Results
// format to the internal struct format.
//
// The receiver is cleared and its fields overwritten.
func (r *JSONTestResults) ConvertFromJSON(ctx context.Context, reader io.Reader) error {
	*r = JSONTestResults{}
	if err := json.NewDecoder(reader).Decode(r); err != nil {
		return err
	}

	// Convert Tests and return.
	if err := r.convertTests("", r.TestsRaw); err != nil {
		return err
	}
	return nil
}

// ToProtos converts test results in r []*pb.TestResult and updates inv
// in-place accordingly.
// If an error is returned, inv is left unchanged.
//
// Uses outputs, the isolated outputs associated with the task, to populate
// artifacts.
// Does not populate TestResult.Name; that happens server-side on RPC response.
func (r *JSONTestResults) ToProtos(ctx context.Context, testIDPrefix string, inv *pb.Invocation, outputs map[string]*pb.Artifact) ([]*pb.TestResult, error) {
	if r.Version != 3 {
		return nil, errors.Reason("unknown JSON Test Results version %d", r.Version).Err()
	}

	// Sort the test name to make the output deterministic.
	testNames := make([]string, 0, len(r.Tests))
	for name := range r.Tests {
		testNames = append(testNames, name)
	}
	sort.Strings(testNames)

	ret := make([]*pb.TestResult, 0, len(r.Tests))
	buf := &strings.Builder{}
	for _, name := range testNames {
		testID := testIDPrefix + name

		// Populate protos.
		unresolvedOutputs, err := r.Tests[name].toProtos(ctx, &ret, testID, outputs, buf)
		if err != nil {
			return nil, errors.Annotate(err, "test %q failed to convert run fields", name).Err()
		}

		// If any outputs cannot be processed, don't cause the rest of processing to fail, but do log.
		if len(unresolvedOutputs) > 0 {
			logging.Errorf(ctx,
				"Test %s could not generate artifact protos for the following:\n%s",
				testID,
				artifactsToString(unresolvedOutputs))
		}
	}

	// Get tags from metadata if any.
	tags, err := r.extractTags()
	if err != nil {
		return nil, err
	}

	// The code below does not return errors, so it is safe to make in-place
	// modifications of inv.
	inv.Interrupted = r.Interrupted

	inv.Tags = append(inv.Tags, pbutil.StringPair(OriginalFormatTagKey, FormatJTR))
	for _, tag := range tags {
		inv.Tags = append(inv.Tags, pbutil.StringPair("json_format_tag", tag))
	}
	if r.BuildNumber != "" {
		inv.Tags = append(inv.Tags, pbutil.StringPair("build_number", r.BuildNumber))
	}

	pbutil.NormalizeInvocation(inv)
	return ret, nil
}

// convertTests converts the trie of tests.
func (r *JSONTestResults) convertTests(curPath string, curNode json.RawMessage) error {
	// curNode should certainly be a map.
	var maybeNode map[string]json.RawMessage
	if err := json.Unmarshal(curNode, &maybeNode); err != nil {
		return errors.Annotate(err, "%q not map[string]json.RawMessage", curNode).Err()
	}

	// Convert the tree.
	for key, value := range maybeNode {
		// Set up test id.
		delim := "/"
		testID := key
		if r.PathDelimiter != "" {
			delim = r.PathDelimiter
		}

		if curPath != "" {
			testID = fmt.Sprintf("%s%s%s", curPath, delim, key)
		} else {
			if prefixJSON, ok := r.Metadata[testNamePrefixKey]; ok {
				var prefix string
				if err := json.Unmarshal(prefixJSON, &prefix); err != nil {
					return errors.Annotate(err, "%s not string, got %q", testNamePrefixKey, prefixJSON).Err()
				}
				testID = prefix + key
			}
		}

		// Try to unmarshal value to TestFields. We check success by checking fields we expect to
		// be populated.
		maybeFields := &TestFields{}
		json.Unmarshal(value, maybeFields)
		// TODO(crbug/1034025): Check maybeFields.Expected is populated.
		if maybeFields.Actual != "" {
			if err := maybeFields.convertArtifacts(); err != nil {
				return errors.Annotate(err,
					"artifacts in %q neither map[string][]string nor map[string]string", value).Err()
			}

			if r.Tests == nil {
				r.Tests = make(map[string]*TestFields)
			}
			r.Tests[testID] = maybeFields
			continue
		}

		// Otherwise, try to process it as an intermediate node.
		if err := r.convertTests(testID, value); err != nil {
			return errors.Annotate(err, "error attempting conversion of %q as intermediate node", value).Err()
		}
	}
	return nil
}

// convertArtifacts converts the raw artifacts into the supported
// map[string][]string representation.
// It also accepts but does not store artifacts in the map[string]string
// representation used by WPT results.
// TODO(crbug/1034021): Support only map[string][]string and remove (unmarshal directly).
func (f *TestFields) convertArtifacts() error {
	for name, arts := range f.ArtifactsRaw {
		var asPathsErr, asStringErr error

		// Try interpreting the artifacts as both formats in turn.
		// Store only the ones that are in the expected map[string][]string format.
		var maybePaths []string
		if asPathsErr = json.Unmarshal(arts, &maybePaths); asPathsErr == nil {
			if f.Artifacts == nil {
				f.Artifacts = map[string][]string{}
			}
			f.Artifacts[name] = maybePaths
			continue
		}

		// Accept string formats, but do not store the converted artifacts, because
		// they are not actually supported by JSON Test Results.
		var maybeString string
		if asStringErr = json.Unmarshal(arts, &maybeString); asStringErr == nil {
			continue
		}

		if asPathsErr != nil || asStringErr != nil {
			return errors.Annotate(errors.NewMultiError(asStringErr, asPathsErr),
				"converting artifacts for %s", name).Err()
		}
	}

	return nil
}

// extractTags tries to read the optional "tags" field in "metadata" as a slice of strings.
func (r *JSONTestResults) extractTags() ([]string, error) {
	maybeTags, ok := r.Metadata["tags"]
	if !ok {
		return nil, nil
	}

	var tags []string
	if err := json.Unmarshal(maybeTags, &tags); err != nil {
		return nil, errors.Annotate(err, "tags not []string, got %q", maybeTags).Err()
	}

	return tags, nil
}

func fromJSONStatus(s string) (pb.TestStatus, error) {
	switch s {
	case "CRASH":
		return pb.TestStatus_CRASH, nil
	case "FAIL":
		return pb.TestStatus_FAIL, nil
	case "PASS":
		return pb.TestStatus_PASS, nil
	case "SKIP":
		return pb.TestStatus_SKIP, nil
	case "TIMEOUT":
		return pb.TestStatus_ABORT, nil

	// The below are web test-specific statuses. They are officially deprecated, but in practice
	// still generated by the tests and should be converted.
	case "IMAGE", "TEXT", "IMAGE+TEXT", "AUDIO", "LEAK", "MISSING":
		return pb.TestStatus_FAIL, nil

	default:
		// There are a number of web test-specific statuses not handled here as they are deprecated.
		return 0, errors.Reason("unknown or unexpected JSON Test Format status %s", s).Err()
	}
}

// toProtos converts the TestFields into zero or more pb.TestResult and
// appends them to dest.
//
// TODO(crbug/1032779): Track artifacts that did not get processed.
func (f *TestFields) toProtos(ctx context.Context, dest *[]*pb.TestResult, testID string, outputs map[string]*pb.Artifact, buf *strings.Builder) (map[string][]string, error) {
	// Process statuses.
	actualStatuses := strings.Split(f.Actual, " ")
	expectedSet := stringset.NewFromSlice(strings.Split(f.Expected, " ")...)

	// TODO(crbug/1034025): Remove.
	if len(expectedSet) == 0 {
		expectedSet.Add("PASS")
	}

	// Process times.
	// Time and Times are both optional, but if Times is present, its length should match the number
	// of runs. Otherwise we have only Time as the duration of the first run.
	if len(f.Times) > 0 && len(f.Times) != len(actualStatuses) {
		return nil, errors.Reason(
			"%d durations populated but has %d test statuses; should match",
			len(f.Times), len(actualStatuses)).Err()
	}

	var durations []float64
	if len(f.Times) > 0 {
		durations = f.Times
	} else if f.Time != 0 { // Do not set duration if it is unknown.
		durations = []float64{f.Time}
	}

	// Get artifacts.
	// We expect that if we have any artifacts, the number of runs from deriving the artifacts
	// should match the number of actual runs. Because the arts are a map from run index to
	// *pb.Artifacts slice, we will not error if artifacts are missing for a run, but log a warning
	// in case the number of runs do not match each other for further investigation.
	arts, unresolved := f.getArtifacts(outputs)
	if len(arts) > 0 && len(actualStatuses) != len(arts) {
		logging.Infof(ctx,
			"Test %s generated %d statuses (%v); does not match number of runs generated from artifacts (%d)",
			testID, len(actualStatuses), actualStatuses, len(arts))
	}

	// Populate protos.
	for i, runStatus := range actualStatuses {
		status, err := fromJSONStatus(runStatus)
		if err != nil {
			return nil, err
		}

		tr := &pb.TestResult{
			TestId:   testID,
			Expected: expectedSet.Has(runStatus),
			Status:   status,
			Tags:     pbutil.StringPairs("json_format_status", runStatus),
		}

		if container, ok := arts[i]; ok {
			buf.Reset()
			err := summaryTmpl.ExecuteTemplate(buf, "jtr", map[string]interface{}{
				"links": container.links,
			})
			if err != nil {
				return nil, err
			}
			tr.SummaryHtml = buf.String()
			// TODO(crbug.com/1071258): associate artifacts with the test result.
		}

		if i < len(durations) {
			tr.Duration = secondsToDuration(durations[i])
		}

		pbutil.NormalizeTestResult(tr)
		*dest = append(*dest, tr)
	}

	return unresolved, nil
}

// testArtifactsPerRun maps a run index to artifacts and links.
type testArtifactsPerRun map[int]*parsedArtifacts

type parsedArtifacts struct {
	artifacts []*pb.Artifact
	links     map[string]string
}

// getArtifacts gets pb.Artifacts corresponding to the TestField's artifacts.
//
// It tries to derive the pb.Artifacts in the following order:
//   - look for them in the isolated outputs represented as pb.Artifacts
//   - check if they're a known special case
//   - fail to process and mark them as `unresolvedArtifacts`
func (f *TestFields) getArtifacts(outputs map[string]*pb.Artifact) (artifacts testArtifactsPerRun, unresolvedArtifacts map[string][]string) {
	artifacts = testArtifactsPerRun{}
	unresolvedArtifacts = map[string][]string{}

	for name, paths := range f.Artifacts {
		for i, path := range paths {
			// We don't use the normalized path everywhere, e.g. if "path" is actually a URL.
			normPath := util.NormalizeIsolatedPath(path)

			// Get the run ID of the artifact. Defaults to 0 (i.e. assumes there is only one run).
			runID, err := artifactRunID(normPath)
			if err != nil {
				unresolvedArtifacts[name] = append(unresolvedArtifacts[name], path)
				continue
			}
			container := artifacts[runID]
			if container == nil {
				container = &parsedArtifacts{
					links: map[string]string{},
				}
				artifacts[runID] = container
			}

			// Look for the path in isolated outputs.
			// TODO(crbug/1032779): Track outputs that were processed.
			if _, art := checkIsolatedOutputs(outputs, normPath); art != nil {
				container.artifacts = append(container.artifacts, art)
				continue
			}

			// If the name is otherwise understood by ResultDB, process it.

			// Accept Gold triage links.
			if name == "gold_triage_link" || name == "triage_link_for_entire_cl" {
				// We don't expect more than one triage link per test run, but if there is more than one,
				// suffix the name with index to ensure we retain it too.
				linkName := name
				if i > 0 {
					linkName = fmt.Sprintf("%s_%d", name, i)
				}
				container.links[linkName] = path
				continue
			}

			// Otherwise, could not populate artifact, so mark it as unresolved.
			unresolvedArtifacts[name] = append(unresolvedArtifacts[name], path)
		}
	}

	return
}

// artifactRunID extracts a run ID, defaulting to 0, or error if it doesn't recognize the format.
func artifactRunID(path string) (int, error) {
	if m := testRunSubdirRe.FindStringSubmatch(path); m != nil {
		return strconv.Atoi(m[1])
	}

	// No retry_<i> subdirectory, so assume it's the first/0th run.
	return 0, nil
}

// artifactsToString converts the given name->paths artifacts map to a string for logging.
func artifactsToString(arts map[string][]string) string {
	names := make([]string, 0, len(arts))
	for name := range arts {
		names = append(names, name)
	}
	sort.Strings(names)

	var msg bytes.Buffer
	w := &indented.Writer{Writer: &msg}
	for _, name := range names {
		fmt.Fprintln(w, name)
		w.Level++
		for _, p := range arts[name] {
			fmt.Fprintln(w, p)
		}
		w.Level--
	}
	return msg.String()
}

// checkIsolatedOutputs looks for the given output path in the isolated output directory.
// Checks the root directory as well as known possible subdirectories.
func checkIsolatedOutputs(outputs map[string]*pb.Artifact, normPath string) (string, *pb.Artifact) {
	// Check root.
	if art, ok := outputs[normPath]; ok {
		return normPath, art
	}

	// Check known candidate subdirectories.
	// TODO(1027708,1031296): Remove.
	for _, dir := range artifactDirectories {
		key := path.Join(dir, normPath)
		if art, ok := outputs[key]; ok {
			return key, art
		}
	}

	return "", nil
}
