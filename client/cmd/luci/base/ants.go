// Copyright 2026 The LUCI Authors.
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

package base

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// DefaultAntsCLIPath is the standard BinFS path to the ants_cli executable.
//
// Deprecated: AnTS test results are now resolved directly via the AnTS v4 OnePlatform API.
const DefaultAntsCLIPath = "/google/bin/releases/android/ants_cli/ants_cli"

// DefaultAntsBaseURL is the default AnTS v4 OnePlatform API endpoint.
const DefaultAntsBaseURL = "https://androidteststorage.pa.googleapis.com"

// AntsUserProject is the shared GCP quota project used for AnTS v4 API billing and quotas.
//
// When calling androidteststorage.pa.googleapis.com with user OAuth tokens (e.g., from
// sso-cred-helper bound to the generic CorpLogin SSO project 913411032810, luci-auth, or ADC),
// OnePlatform requires X-Goog-User-Project to attribute quota to a consumer project that has
// androidteststorage.pa.googleapis.com enabled and grants roles/serviceusage.serviceUsageConsumer
// (see go/quota-project-override). android-build-fetch-artifact is the standard shared human
// quota project for Android Build / AnTS v4 CLI tools.
const AntsUserProject = "android-build-fetch-artifact"

var (
	antsTRRegex  = regexp.MustCompile(`^(?i)TR[0-9]+$`)
	antsInvRegex = regexp.MustCompile(`^(?:I[0-9]+|ants-[Ii][0-9]+)$`)
	antsWURegex  = regexp.MustCompile(`^(?:WU[0-9]+|ants-[Ww][Uu][0-9]+)$`)
)

type antsContextKeyType struct{}
type authOptsContextKeyType struct{}

type antsContextConfig struct {
	client  *http.Client
	baseURL string
}

// WithAntsHTTPClient returns a context with a custom HTTP client and optional base URL for AnTS API calls.
func WithAntsHTTPClient(ctx context.Context, client *http.Client, baseURL string) context.Context {
	return context.WithValue(ctx, antsContextKeyType{}, antsContextConfig{
		client:  client,
		baseURL: baseURL,
	})
}

// WithAuthOptions returns a context with custom auth.Options to use when creating authenticated clients.
func WithAuthOptions(ctx context.Context, opts auth.Options) context.Context {
	return context.WithValue(ctx, authOptsContextKeyType{}, opts)
}

// IsAntsTestResultID returns true if s is an AnTS test result ID (e.g. TR12345678).
func IsAntsTestResultID(s string) bool {
	return antsTRRegex.MatchString(strings.TrimSpace(s))
}

// IsAntsInvocationID returns true if s is an AnTS invocation ID (e.g. I12345678).
func IsAntsInvocationID(s string) bool {
	return antsInvRegex.MatchString(strings.TrimSpace(s))
}

// IsAntsWorkUnitID returns true if s is an AnTS work unit ID (e.g. WU12345678).
func IsAntsWorkUnitID(s string) bool {
	return antsWURegex.MatchString(strings.TrimSpace(s))
}

// IsAntsURL returns true if raw is an Android Test Investigate (ATI) URL.
func IsAntsURL(raw string) bool {
	clean := strings.TrimSpace(raw)
	return strings.Contains(clean, "android-build.corp.google.com") ||
		strings.Contains(clean, "android-build.googleplex.com") ||
		strings.Contains(clean, "/test_investigate/")
}

// ExtractAntsURLComponents extracts the invocation ID and test result ID from an ATI URL.
func ExtractAntsURLComponents(raw string) (invID, trID string) {
	clean := strings.TrimSpace(raw)

	// Check query parameters first (e.g. ?invocationId=I123, ?testResultId=TR123)
	if u, err := url.Parse(clean); err == nil {
		q := u.Query()
		for _, key := range []string{"invocationId", "invocation_id", "invId", "inv_id"} {
			if v := q.Get(key); v != "" && IsAntsInvocationID(v) {
				invID = v
				break
			}
		}
		for _, key := range []string{"testResultId", "test_result_id", "testId", "trId", "tr_id"} {
			if v := q.Get(key); v != "" && IsAntsTestResultID(v) {
				trID = v
				break
			}
		}
	}

	if idx := strings.IndexAny(clean, "?#"); idx != -1 {
		clean = clean[:idx]
	}

	// Match /invocation/(I[0-9]+) if not already extracted
	if invID == "" {
		if idx := strings.Index(clean, "/invocation/"); idx != -1 {
			after := clean[idx+len("/invocation/"):]
			parts := strings.Split(after, "/")
			if len(parts) > 0 && IsAntsInvocationID(parts[0]) {
				invID = parts[0]
			}
		}
	}

	// Match /test/(TR[0-9]+) if not already extracted
	if trID == "" {
		if idx := strings.Index(clean, "/test/"); idx != -1 {
			after := clean[idx+len("/test/"):]
			parts := strings.Split(after, "/")
			if len(parts) > 0 && IsAntsTestResultID(parts[0]) {
				trID = parts[0]
			}
		}
	}

	return invID, trID
}

// AntsProperty represents a key-value property in AnTS metadata.
type AntsProperty struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// AntsTestResultInfo contains parsed details from an AnTS test result.
type AntsTestResultInfo struct {
	TestResultID     string         `json:"test_result_id,omitempty"`
	TestCase         string         `json:"test_case,omitempty"`
	Status           string         `json:"status,omitempty"`
	WorkUnitID       string         `json:"work_unit_id,omitempty"`
	InvocationID     string         `json:"invocation_id,omitempty"`
	RunNumber        int            `json:"run_number,omitempty"`
	AttemptNumber    int            `json:"attempt_number,omitempty"`
	TestIdentifierID string         `json:"test_identifier_id,omitempty"`
	ModuleParameters []AntsProperty `json:"module_parameters,omitempty"`
	AggregationType  string         `json:"aggregation_type,omitempty"`

	// Decomposed test case components
	ModuleName    string `json:"module_name,omitempty"`
	ClassName     string `json:"class_name,omitempty"`
	MethodName    string `json:"method_name,omitempty"`
	IsModuleError bool   `json:"is_module_error,omitempty"`
}

// FindAntsCLI locates the ants_cli executable on the system.
//
// Deprecated: AnTS test results are now resolved directly via the AnTS v4 OnePlatform API.
func FindAntsCLI() (string, error) {
	if custom := os.Getenv("ANTS_CLI_PATH"); custom != "" {
		if custom == "disabled" || custom == "none" {
			return "", errors.New("AnTS test result ID resolution is disabled (ANTS_CLI_PATH=" + custom + ")")
		}
		if _, err := os.Stat(custom); err == nil {
			return custom, nil
		}
		if p, err := exec.LookPath(custom); err == nil {
			return p, nil
		}
		return "", errors.Fmt("AnTS test result ID resolution requires ants_cli (not found at %s)", custom)
	}
	if _, err := os.Stat(DefaultAntsCLIPath); err == nil {
		return DefaultAntsCLIPath, nil
	}
	if p, err := exec.LookPath("ants_cli"); err == nil {
		return p, nil
	}
	return "", errors.New("AnTS test result ID resolution requires ants_cli (not found at " + DefaultAntsCLIPath + " or in PATH)")
}

// ResolveAntsTestResult retrieves test result details for a test result ID directly from the AnTS v4 OnePlatform API.
func ResolveAntsTestResult(ctx context.Context, testResultID string) (*AntsTestResultInfo, error) {
	testResultID = strings.TrimSpace(testResultID)
	if !IsAntsTestResultID(testResultID) {
		return nil, errors.Fmt("invalid AnTS test result ID %q (expected format TR<digits>)", testResultID)
	}

	if custom := os.Getenv("ANTS_CLI_PATH"); custom == "disabled" || custom == "none" {
		return nil, errors.New("AnTS test result ID resolution is disabled (ANTS_CLI_PATH=" + custom + ")")
	}

	baseURL := antsBaseURL(ctx)
	httpClient, err := antsHTTPClient(ctx)
	if err != nil {
		return nil, err
	}

	body, err := fetchAntsTestResultJSON(ctx, httpClient, baseURL, testResultID)
	if err != nil {
		return nil, err
	}

	info, err := ParseAntsTestResultJSON(body)
	if err != nil {
		return nil, err
	}
	if info.TestResultID == "" {
		info.TestResultID = testResultID
	}
	return info, nil
}

// antsBaseURL resolves the AnTS v4 API base URL.
//
// Resolution order:
// 1. Custom base URL provided in ctx via WithAntsHTTPClient (used for testing).
// 2. ANTS_BASE_URL environment variable override (used for custom endpoints or proxying).
// 3. DefaultAntsBaseURL (https://androidteststorage.pa.googleapis.com).
func antsBaseURL(ctx context.Context) string {
	baseURL := DefaultAntsBaseURL
	if cfg, ok := ctx.Value(antsContextKeyType{}).(antsContextConfig); ok && cfg.baseURL != "" {
		baseURL = cfg.baseURL
	}
	if envURL := os.Getenv("ANTS_BASE_URL"); envURL != "" {
		baseURL = envURL
	}
	return strings.TrimRight(baseURL, "/")
}

// antsHTTPClient resolves or constructs the HTTP client used for AnTS API calls.
//
// If a custom client was injected into ctx via WithAntsHTTPClient, it is returned directly
// without initiating OAuth authentication (enabling hermetic testing). Otherwise, an
// authenticated HTTP client is created via auth.NewAuthenticator configured with
// scopes.AndroidBuild ("https://www.googleapis.com/auth/androidbuild.internal"),
// scopes.Email, and auth.GoogleADCAllow.
func antsHTTPClient(ctx context.Context) (*http.Client, error) {
	if cfg, ok := ctx.Value(antsContextKeyType{}).(antsContextConfig); ok && cfg.client != nil {
		return cfg.client, nil
	}

	var opts auth.Options
	if ctxOpts, ok := ctx.Value(authOptsContextKeyType{}).(auth.Options); ok {
		opts = ctxOpts
	} else {
		opts = chromeinfra.DefaultAuthOptions()
	}
	opts.Scopes = []string{scopes.AndroidBuild, scopes.Email}
	if opts.GoogleADCPolicy == "" {
		opts.GoogleADCPolicy = auth.GoogleADCAllow
	}

	client, err := auth.NewAuthenticator(ctx, auth.OptionalLogin, opts).Client()
	if err != nil {
		return nil, errors.Fmt("failed to create authenticated HTTP client: %w", err)
	}
	return client, nil
}

// fetchAntsTestResultJSON executes GET /v4/tests/{test_result_id} against the AnTS v4 OnePlatform API.
func fetchAntsTestResultJSON(ctx context.Context, httpClient *http.Client, baseURL, testResultID string) ([]byte, error) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	reqURL := fmt.Sprintf("%s/v4/tests/%s", baseURL, url.PathEscape(testResultID))
	req, err := http.NewRequestWithContext(callCtx, "GET", reqURL, nil)
	if err != nil {
		return nil, errors.Fmt("failed to create AnTS API request: %w", err)
	}
	req.Header.Set("X-Goog-User-Project", AntsUserProject)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.Fmt("AnTS API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, errors.Fmt("failed to read AnTS API response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, formatAntsAPIError(testResultID, resp.StatusCode, body)
	}
	return body, nil
}

// formatAntsAPIError formats a user-friendly error for non-200 AnTS API responses.
//
// Specific mappings:
// - HTTP 404 indicates the test result ID does not exist in AnTS.
// - HTTP 401/403 indicate missing or insufficient credentials for the AndroidBuild OnePlatform API.
// - Other status codes return the upstream error message or status code.
func formatAntsAPIError(testResultID string, statusCode int, body []byte) error {
	msg := parseOnePlatformErrorMessage(body)
	switch statusCode {
	case http.StatusNotFound:
		if msg != "" {
			return errors.Fmt("AnTS test result %q not found (HTTP 404): %s", testResultID, msg)
		}
		return errors.Fmt("AnTS test result %q not found (HTTP 404)", testResultID)
	case http.StatusUnauthorized, http.StatusForbidden:
		if msg != "" {
			return errors.Fmt("AnTS API permission denied for test result %q (HTTP %d): %s", testResultID, statusCode, msg)
		}
		return errors.Fmt("AnTS API permission denied for test result %q (HTTP %d)", testResultID, statusCode)
	default:
		if msg != "" {
			return errors.Fmt("AnTS API request failed with HTTP %d: %s", statusCode, msg)
		}
		return errors.Fmt("AnTS API request failed with HTTP %d", statusCode)
	}
}

func parseOnePlatformErrorMessage(body []byte) string {
	var errResp struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		return errResp.Error.Message
	}
	return strings.TrimSpace(string(body))
}

// antsV4TestResultResponse matches the proto3 JSON schema of
// google.internal.android.testresults.ants.v4.TestResult returned by OnePlatform API.
// OnePlatform v4 proto3 JSON strictly uses camelCase field names.
type antsV4TestResultResponse struct {
	TestResultID      string                   `json:"testResultId"`
	InvocationID      string                   `json:"invocationId"`
	WorkUnitID        string                   `json:"workUnitId"`
	TestStatus        string                   `json:"testStatus"`
	RunNumber         int                      `json:"runNumber"`
	AttemptNumber     int                      `json:"attemptNumber"`
	TestIdentifierID  string                   `json:"testIdentifierId"`
	TestIdentifier    *antsV4TestIdentifier    `json:"testIdentifier"`
	AggregationDetail *antsV4AggregationDetail `json:"aggregationDetail"`
}

// antsV4TestIdentifier matches google.internal.android.testresults.ants.v4.TestIdentifier.
type antsV4TestIdentifier struct {
	Module           string         `json:"module"`
	ModuleParameters []AntsProperty `json:"moduleParameters"`
	TestClass        string         `json:"testClass"`
	Method           string         `json:"method"`
}

// antsV4AggregationDetail matches google.internal.android.testresults.ants.v4.AggregationDetail.
// In the AnTS v4 proto schema, the field is aggregation_level (proto3 JSON: aggregationLevel),
// while some services and documentation also refer to aggregationType. Both are accepted.
type antsV4AggregationDetail struct {
	AggregationType  string `json:"aggregationType"`
	AggregationLevel string `json:"aggregationLevel"`
}

// ParseAntsTestResultJSON unmarshals AnTS v4 OnePlatform TestResult JSON response into AntsTestResultInfo.
func ParseAntsTestResultJSON(data []byte) (*AntsTestResultInfo, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("empty AnTS test result JSON response")
	}

	var raw antsV4TestResultResponse
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, errors.Fmt("failed to unmarshal AnTS v4 TestResult JSON: %w", err)
	}

	trID := raw.TestResultID
	invID := raw.InvocationID
	wuID := raw.WorkUnitID

	if trID == "" && invID == "" && raw.TestIdentifier == nil && raw.TestStatus == "" && raw.AggregationDetail == nil {
		return nil, errors.New("invalid AnTS v4 TestResult JSON: no recognizable fields found")
	}

	info := &AntsTestResultInfo{
		TestResultID:     trID,
		InvocationID:     invID,
		WorkUnitID:       wuID,
		RunNumber:        raw.RunNumber,
		AttemptNumber:    raw.AttemptNumber,
		TestIdentifierID: raw.TestIdentifierID,
		Status:           normalizeAntsStatus(raw.TestStatus),
	}

	if raw.AggregationDetail != nil {
		if raw.AggregationDetail.AggregationType != "" {
			info.AggregationType = raw.AggregationDetail.AggregationType
		} else {
			info.AggregationType = raw.AggregationDetail.AggregationLevel
		}
	}

	if raw.TestIdentifier != nil {
		info.ModuleName = raw.TestIdentifier.Module
		info.ClassName = raw.TestIdentifier.TestClass
		info.MethodName = raw.TestIdentifier.Method
		if len(raw.TestIdentifier.ModuleParameters) > 0 {
			info.ModuleParameters = raw.TestIdentifier.ModuleParameters
		}
	}

	info.IsModuleError = isAntsModuleError(info.AggregationType, info.ClassName, info.MethodName)
	info.TestCase = synthesizeAntsTestCase(info.ModuleName, info.ClassName, info.MethodName)

	return info, nil
}

// isAntsModuleError determines whether an AnTS test result represents a module-level failure
// (such as runner crash, setup failure, or build error) rather than a method test verdict.
func isAntsModuleError(aggregationType, className, methodName string) bool {
	aggUpper := strings.ToUpper(strings.TrimSpace(aggregationType))
	if aggUpper != "" && aggUpper != "METHOD" && aggUpper != "AGGREGATION_LEVEL_UNSPECIFIED" {
		return true
	}
	return className == "" && methodName == ""
}

// synthesizeAntsTestCase synthesizes the legacy test case string for backwards compatibility
// with `ids.go` and callers expecting the `ants_cli show-test` format:
// - Full method result: "<module>#<testClass>.<method>"
// - Module-level error aggregate: "<module>#."
// - Invocation-level placeholder / runner failure without module: "#."
// - Partial results without testClass or without method.
func synthesizeAntsTestCase(moduleName, className, methodName string) string {
	if className == "" && methodName == "" {
		if moduleName != "" {
			return moduleName + "#."
		}
		return "#."
	}
	if className != "" && methodName != "" {
		return fmt.Sprintf("%s#%s.%s", moduleName, className, methodName)
	}
	if className == "" && methodName != "" {
		return fmt.Sprintf("%s#%s", moduleName, methodName)
	}
	return fmt.Sprintf("%s#%s", moduleName, className)
}

func normalizeAntsStatus(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "PASS":
		return "pass"
	case "FAIL":
		return "fail"
	case "IGNORED":
		return "ignored"
	case "ASSUMPTION_FAILURE":
		return "assumptionFailure"
	case "TEST_ERROR":
		return "testError"
	case "TEST_SKIPPED":
		return "testSkipped"
	default:
		return strings.TrimSpace(raw)
	}
}

// ParseAntsShowTestOutput parses the output of `ants_cli show-test` (supporting both JSON and key-value text format).
func ParseAntsShowTestOutput(out string) (*AntsTestResultInfo, error) {
	trimmed := strings.TrimSpace(out)
	if strings.HasPrefix(trimmed, "{") {
		var jsonInfo AntsTestResultInfo
		if err := json.Unmarshal([]byte(trimmed), &jsonInfo); err == nil && (jsonInfo.TestResultID != "" || jsonInfo.TestCase != "") {
			if jsonInfo.ModuleName == "" && jsonInfo.TestCase != "" {
				deconstructTestCase(&jsonInfo)
			}
			return &jsonInfo, nil
		}
		if info, err := ParseAntsTestResultJSON([]byte(trimmed)); err == nil && (info.TestResultID != "" || info.TestCase != "" || info.InvocationID != "") {
			return info, nil
		}
	}

	info := &AntsTestResultInfo{}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Test Result ID:") {
			info.TestResultID = strings.TrimSpace(strings.TrimPrefix(line, "Test Result ID:"))
		} else if strings.HasPrefix(line, "Test Case:") {
			info.TestCase = strings.TrimSpace(strings.TrimPrefix(line, "Test Case:"))
		} else if strings.HasPrefix(line, "Status:") {
			info.Status = strings.TrimSpace(strings.TrimPrefix(line, "Status:"))
		} else if strings.HasPrefix(line, "Work Unit ID:") {
			info.WorkUnitID = strings.TrimSpace(strings.TrimPrefix(line, "Work Unit ID:"))
		} else if strings.HasPrefix(line, "Invocation ID:") {
			info.InvocationID = strings.TrimSpace(strings.TrimPrefix(line, "Invocation ID:"))
		} else if strings.HasPrefix(line, "Run Number:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Run Number:"))
			info.RunNumber, _ = strconv.Atoi(val)
		} else if strings.HasPrefix(line, "Attempt Number:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Attempt Number:"))
			info.AttemptNumber, _ = strconv.Atoi(val)
		} else if strings.HasPrefix(line, "Test Identifier ID:") {
			info.TestIdentifierID = strings.TrimSpace(strings.TrimPrefix(line, "Test Identifier ID:"))
		}
	}

	if info.TestResultID == "" && info.TestCase == "" && info.InvocationID == "" {
		return nil, errors.New("failed to parse ants_cli show-test output: no test result details found")
	}

	deconstructTestCase(info)
	return info, nil
}

func deconstructTestCase(info *AntsTestResultInfo) {
	if info == nil {
		return
	}
	tc := info.TestCase
	if idx := strings.Index(tc, "#"); idx != -1 {
		info.ModuleName = tc[:idx]
		rest := tc[idx+1:]
		if rest == "." || rest == "" {
			info.IsModuleError = true
		} else if lastDot := strings.LastIndex(rest, "."); lastDot != -1 {
			info.ClassName = rest[:lastDot]
			info.MethodName = rest[lastDot+1:]
		} else {
			info.MethodName = rest
		}
	} else if tc != "" {
		info.ModuleName = tc
		info.IsModuleError = true
	}
}
