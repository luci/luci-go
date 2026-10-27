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
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

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

// AndroidAccessHint explains the MDB group requirements for accessing Android
// Build (AnTS) and Android ResultDB test data.
const AndroidAccessHint = "accessing Android (AnTS / ResultDB) test data requires membership in mdb/android-build-access for users (http://go/android-build-access) or mdb/android-build-access-agent-identities for agent identities (http://go/android-agent-identity-access)"

var (
	antsTRRegex  = regexp.MustCompile(`^(?i)TR[0-9]+$`)
	antsInvRegex = regexp.MustCompile(`^(?:I[0-9]+|ants-[Ii][0-9]+)$`)
	// antsWURegex matches AnTS work unit IDs in bare ("WU123"), ResultDB-prefixed
	// ("ants-wu123"), attempt-suffixed ("ants-wu123-attempt__0"), or composite
	// ("ants-wu123:u-<uuid>", "ants-wu123-attempt__0:u-<uuid>") forms.
	// Capture group 1 holds the numeric work unit digits; capture group 2 holds
	// any trailing "-attempt__<N>" and/or ":u-<uuid>" suffix.
	antsWURegex = regexp.MustCompile(`^(?i)(?:ants-)?wu([0-9]+)((?:-attempt__[0-9]+)?(?::u-[0-9a-fA-F-]+)?)$`)
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

// IsAntsWorkUnitID returns true if s is an AnTS work unit ID (e.g. WU12345678,
// ants-wu12345678, ants-wu12345678:u-abc, or ants-wu12345678-attempt__0:u-abc).
func IsAntsWorkUnitID(s string) bool {
	return antsWURegex.MatchString(strings.TrimSpace(s))
}

// ExtractBareAntsWorkUnitID extracts the canonical uppercase AnTS work unit ID
// ("WU<digits>") from any supported AnTS work unit representation (e.g.,
// "WU123", "ants-wu123", "workUnits/WU123", "ants-wu123-attempt__0", or
// "ants-wu123-attempt__0:u-1234abcd"), or returns "" if s is not an AnTS work
// unit ID.
func ExtractBareAntsWorkUnitID(s string) string {
	clean := strings.TrimSpace(s)
	if idx := strings.LastIndex(clean, "workUnits/"); idx != -1 {
		clean = clean[idx+len("workUnits/"):]
	}
	m := antsWURegex.FindStringSubmatch(clean)
	if len(m) == 0 {
		return ""
	}
	return "WU" + m[1]
}

// IsAntsURL returns true if raw is an Android Test Investigate (ATI) URL.
func IsAntsURL(raw string) bool {
	clean := strings.TrimSpace(raw)
	return strings.Contains(clean, "android-build.corp.google.com") ||
		strings.Contains(clean, "android-build.googleplex.com") ||
		strings.Contains(clean, "/test_investigate/")
}

// extractURLParamOrPath extracts a matching identifier from either URL query
// parameters (checked first in order of queryKeys) or URL path segments
// immediately following any of pathPrefixes.
func extractURLParamOrPath(raw string, queryKeys, pathPrefixes []string, match func(string) bool) string {
	clean := strings.TrimSpace(raw)
	if u, err := url.Parse(clean); err == nil {
		q := u.Query()
		for _, key := range queryKeys {
			if v := q.Get(key); v != "" && match(v) {
				return v
			}
		}
	}
	if idx := strings.IndexAny(clean, "?#"); idx != -1 {
		clean = clean[:idx]
	}
	for _, prefix := range pathPrefixes {
		if idx := strings.Index(clean, prefix); idx != -1 {
			after := clean[idx+len(prefix):]
			parts := strings.Split(after, "/")
			if len(parts) > 0 && match(parts[0]) {
				return parts[0]
			}
		}
	}
	return ""
}

// ExtractAntsURLComponents extracts the invocation ID and test result ID from an ATI URL.
func ExtractAntsURLComponents(raw string) (invID, trID string) {
	invID = extractURLParamOrPath(
		raw,
		[]string{"invocationId", "invocation_id", "invId", "inv_id"},
		[]string{"/invocation/"},
		IsAntsInvocationID,
	)
	trID = extractURLParamOrPath(
		raw,
		[]string{"testResultId", "test_result_id", "testId", "trId", "tr_id"},
		[]string{"/test/"},
		IsAntsTestResultID,
	)
	return invID, trID
}

// ExtractAntsURLWorkUnit extracts an AnTS work unit ID from an ATI URL's query
// parameters or path segments, if present.
func ExtractAntsURLWorkUnit(raw string) string {
	return extractURLParamOrPath(
		raw,
		[]string{"workUnitId", "work_unit_id", "wuId", "wu_id"},
		[]string{"/workunit/", "/work_unit/", "/workUnit/"},
		IsAntsWorkUnitID,
	)
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

// ResolveAntsTestResult retrieves test result details for a test result ID directly from the AnTS v4 OnePlatform API.
func ResolveAntsTestResult(ctx context.Context, testResultID string) (*AntsTestResultInfo, error) {
	testResultID = strings.TrimSpace(testResultID)
	if !IsAntsTestResultID(testResultID) {
		return nil, errors.Fmt("invalid AnTS test result ID %q (expected format TR<digits>)", testResultID)
	}

	baseURL := antsBaseURL(ctx)
	httpClient, err := antsHTTPClient(ctx)
	if err != nil {
		return nil, err
	}

	body, err := fetchAntsResourceJSON(ctx, httpClient, baseURL, "tests", "test result", testResultID)
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

// fetchAntsResourceJSON executes GET /v4/{collection}/{resource_id} against the
// AnTS v4 OnePlatform API with the required quota project header and a 30s
// request timeout.
func fetchAntsResourceJSON(ctx context.Context, httpClient *http.Client, baseURL, collection, resourceKind, resourceID string) ([]byte, error) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	reqURL := fmt.Sprintf("%s/v4/%s/%s", baseURL, collection, url.PathEscape(resourceID))
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
		return nil, formatAntsResourceAPIError(resourceKind, resourceID, resp.StatusCode, body)
	}
	return body, nil
}

// formatAntsResourceAPIError formats a user-friendly error for non-200 AnTS API responses.
//
// Specific mappings:
// - HTTP 404 indicates the requested resource ID does not exist in AnTS.
// - HTTP 401/403 indicate missing or insufficient credentials for the AndroidBuild OnePlatform API.
// - Other status codes return the upstream error message or status code.
func formatAntsResourceAPIError(resourceKind, resourceID string, statusCode int, body []byte) error {
	msg := parseOnePlatformErrorMessage(body)
	switch statusCode {
	case http.StatusNotFound:
		if msg != "" {
			return errors.Fmt("AnTS %s %q not found (HTTP 404): %s", resourceKind, resourceID, msg)
		}
		return errors.Fmt("AnTS %s %q not found (HTTP 404)", resourceKind, resourceID)
	case http.StatusUnauthorized, http.StatusForbidden:
		if msg != "" {
			return errors.Fmt("AnTS API permission denied for %s %q (HTTP %d): %s (%s)", resourceKind, resourceID, statusCode, msg, AndroidAccessHint)
		}
		return errors.Fmt("AnTS API permission denied for %s %q (HTTP %d) (%s)", resourceKind, resourceID, statusCode, AndroidAccessHint)
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

// AntsWorkUnitInfo contains parsed details from an AnTS work unit.
type AntsWorkUnitInfo struct {
	WorkUnitID               string         `json:"work_unit_id,omitempty"`
	InvocationID             string         `json:"invocation_id,omitempty"`
	ParentID                 string         `json:"parent_id,omitempty"`
	Name                     string         `json:"name,omitempty"`
	Type                     string         `json:"type,omitempty"`
	State                    string         `json:"state,omitempty"`
	ModuleName               string         `json:"module_name,omitempty"`
	ModuleParameters         []AntsProperty `json:"module_parameters,omitempty"`
	Properties               []AntsProperty `json:"properties,omitempty"`
	ResultDBRootInvocationID string         `json:"resultdb_root_invocation_id,omitempty"`
	ResultDBWorkUnitID       string         `json:"resultdb_work_unit_id,omitempty"`
	CreationTimestamp        int64          `json:"creation_timestamp,omitempty"`
	CompleteTimestamp        int64          `json:"complete_timestamp,omitempty"`
}

// fetchAndParseAntsWorkUnit fetches GET /v4/workunits/{work_unit_id} and
// parses the response into AntsWorkUnitInfo.
func fetchAndParseAntsWorkUnit(ctx context.Context, httpClient *http.Client, baseURL, workUnitID string) (*AntsWorkUnitInfo, error) {
	body, err := fetchAntsResourceJSON(ctx, httpClient, baseURL, "workunits", "work unit", workUnitID)
	if err != nil {
		return nil, err
	}
	return ParseAntsWorkUnitJSON(body)
}

// ResolveAntsWorkUnit retrieves work unit details for an AnTS work unit ID
// directly from the AnTS v4 OnePlatform API (GET /v4/workunits/{work_unit_id}).
//
// If the queried work unit is a child work unit (e.g. TF_TEST_RUN or TF_MODULE)
// that does not directly carry `resultdb_work_unit_id` or `moduleInfo`, it
// walks up at most 3 ancestor work units via `parentId` to populate the parent
// task's ResultDB work unit ID and enclosing module metadata.
func ResolveAntsWorkUnit(ctx context.Context, workUnitID string) (*AntsWorkUnitInfo, error) {
	bareWUID := ExtractBareAntsWorkUnitID(workUnitID)
	if bareWUID == "" {
		return nil, errors.Fmt("invalid AnTS work unit ID %q (expected format WU<digits> or ants-wu<digits>)", workUnitID)
	}

	baseURL := antsBaseURL(ctx)
	httpClient, err := antsHTTPClient(ctx)
	if err != nil {
		return nil, err
	}

	info, err := fetchAndParseAntsWorkUnit(ctx, httpClient, baseURL, bareWUID)
	if err != nil {
		return nil, err
	}
	if info.WorkUnitID == "" {
		info.WorkUnitID = bareWUID
	}

	// Walk up at most 3 ancestor work units if we are missing the enclosing
	// task's ResultDB work unit ID, invocation ID, or (for TF_TEST_RUN/TF_MODULE)
	// the enclosing module metadata. Skip ancestor walk if the caller already
	// passed a pre-resolved ":u-<uuid>" composite work unit ID.
	if !strings.Contains(strings.ToLower(workUnitID), ":u-") {
		needsModule := strings.EqualFold(info.Type, "TF_TEST_RUN") || strings.EqualFold(info.Type, "TF_MODULE")
		currParentID := ExtractBareAntsWorkUnitID(info.ParentID)
		for depth := 0; depth < 3 && currParentID != ""; depth++ {
			if info.ResultDBWorkUnitID != "" && info.InvocationID != "" && (!needsModule || info.ModuleName != "") {
				break
			}
			parentInfo, pErr := fetchAndParseAntsWorkUnit(ctx, httpClient, baseURL, currParentID)
			if pErr != nil {
				break
			}
			if info.InvocationID == "" && parentInfo.InvocationID != "" {
				info.InvocationID = parentInfo.InvocationID
			}
			if info.ResultDBRootInvocationID == "" && parentInfo.ResultDBRootInvocationID != "" {
				info.ResultDBRootInvocationID = parentInfo.ResultDBRootInvocationID
			}
			if info.ResultDBWorkUnitID == "" && parentInfo.ResultDBWorkUnitID != "" {
				info.ResultDBWorkUnitID = parentInfo.ResultDBWorkUnitID
			}
			if info.ModuleName == "" && parentInfo.ModuleName != "" {
				info.ModuleName = parentInfo.ModuleName
			}
			if len(info.ModuleParameters) == 0 && len(parentInfo.ModuleParameters) > 0 {
				info.ModuleParameters = parentInfo.ModuleParameters
			}
			currParentID = ExtractBareAntsWorkUnitID(parentInfo.ParentID)
		}
	}

	// For TF_TEST_RUN work units where module name is still empty after walking
	// ancestors, extract the module name from the work unit name (e.g.
	// "x86_64 bluetooth_test_gd_unit" -> "bluetooth_test_gd_unit").
	if info.ModuleName == "" && strings.EqualFold(info.Type, "TF_TEST_RUN") && info.Name != "" {
		trimmedName := strings.TrimSpace(info.Name)
		if idx := strings.LastIndex(trimmedName, " "); idx != -1 {
			info.ModuleName = strings.TrimSpace(trimmedName[idx+1:])
		} else {
			info.ModuleName = trimmedName
		}
	}

	return info, nil
}

// antsV4WorkUnitResponse matches the proto3 JSON schema of
// google.internal.android.testresults.ants.v4.WorkUnit returned by OnePlatform API.
type antsV4WorkUnitResponse struct {
	ID           string            `json:"id"`
	WorkUnitID   string            `json:"workUnitId"`
	InvocationID string            `json:"invocationId"`
	ParentID     string            `json:"parentId"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	State        string            `json:"state"`
	ModuleInfo   *antsV4ModuleInfo `json:"moduleInfo"`
	Properties   []AntsProperty    `json:"properties"`
	Timing       *struct {
		CreationTimestamp json.Number `json:"creationTimestamp"`
		CompleteTimestamp json.Number `json:"completeTimestamp"`
	} `json:"timing"`
	ResultDBInfo *struct {
		WorkUnit       string `json:"workUnit"`
		RootInvocation string `json:"rootInvocation"`
	} `json:"resultdbInfo"`
}

// antsV4ModuleInfo matches google.internal.android.testresults.ants.v4.ModuleInfo.
type antsV4ModuleInfo struct {
	Name             string         `json:"name"`
	ModuleParameters []AntsProperty `json:"moduleParameters"`
}

// ParseAntsWorkUnitJSON unmarshals an AnTS v4 OnePlatform WorkUnit JSON
// response into AntsWorkUnitInfo.
func ParseAntsWorkUnitJSON(data []byte) (*AntsWorkUnitInfo, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("empty AnTS work unit JSON response")
	}

	var raw antsV4WorkUnitResponse
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, errors.Fmt("failed to unmarshal AnTS v4 WorkUnit JSON: %w", err)
	}

	wuID := raw.ID
	if wuID == "" {
		wuID = raw.WorkUnitID
	}

	if wuID == "" && raw.InvocationID == "" && raw.Name == "" && raw.Type == "" && raw.State == "" && len(raw.Properties) == 0 {
		return nil, errors.New("invalid AnTS v4 WorkUnit JSON: no recognizable fields found")
	}

	info := &AntsWorkUnitInfo{
		WorkUnitID:   wuID,
		InvocationID: raw.InvocationID,
		ParentID:     raw.ParentID,
		Name:         raw.Name,
		Type:         raw.Type,
		State:        raw.State,
		Properties:   raw.Properties,
	}

	if raw.Timing != nil {
		if ts, err := strconv.ParseInt(string(raw.Timing.CreationTimestamp), 10, 64); err == nil {
			info.CreationTimestamp = ts
		}
		if ts, err := strconv.ParseInt(string(raw.Timing.CompleteTimestamp), 10, 64); err == nil {
			info.CompleteTimestamp = ts
		}
	}

	if raw.ModuleInfo != nil {
		info.ModuleName = raw.ModuleInfo.Name
		if len(raw.ModuleInfo.ModuleParameters) > 0 {
			info.ModuleParameters = raw.ModuleInfo.ModuleParameters
		}
	}

	for _, prop := range raw.Properties {
		switch prop.Name {
		case "resultdb_root_invocation_id":
			info.ResultDBRootInvocationID = strings.TrimSpace(prop.Value)
		case "resultdb_work_unit_id":
			info.ResultDBWorkUnitID = strings.TrimSpace(prop.Value)
		}
	}

	if raw.ResultDBInfo != nil {
		if info.ResultDBRootInvocationID == "" && raw.ResultDBInfo.RootInvocation != "" {
			info.ResultDBRootInvocationID = strings.TrimPrefix(raw.ResultDBInfo.RootInvocation, "rootInvocations/")
		}
		if info.ResultDBWorkUnitID == "" && raw.ResultDBInfo.WorkUnit != "" {
			_, rdbWU := ExtractWorkUnitComponents(raw.ResultDBInfo.WorkUnit)
			if rdbWU != "" {
				info.ResultDBWorkUnitID = rdbWU
			}
		}
	}

	// For TF_MODULE work units without moduleInfo.name, Name is the module name.
	if info.ModuleName == "" && strings.EqualFold(info.Type, "TF_MODULE") && info.Name != "" {
		info.ModuleName = info.Name
	}

	return info, nil
}

// ResolveResultDBAntsWorkUnit resolves an AnTS work unit ID (`WU<digits>` or
// `ants-wu<digits>`) to its actual ResultDB work unit ID within `invID`.
//
// In TradeFed's ResultDB integration, AnTS work units map to ResultDB work
// units according to their level in the execution hierarchy:
//   - Top-level invocation work units (`ParentID == ""`) map to ResultDB's
//     `root` work unit.
//   - Task-level work units (`TFC_COMMAND_TASK`) exist directly in ResultDB as
//     `ants-wu<task_id>` (or `ants-wu<task_id>-attempt__<N>`).
//   - Module (`TF_MODULE`) and test-run (`TF_TEST_RUN`) work units are stored
//     in ResultDB as composite child work units under their parent task work
//     unit with IDs of the form `<task_wu_id>:u-<uuid>`, rather than by their
//     own AnTS `WU<digits>` ID.
//
// Resolution steps:
//  1. If `rawWorkUnitID` already carries a composite `:u-<uuid>` or
//     `-attempt__<N>` suffix, return its normalized form immediately.
//  2. Probe `GetWorkUnit(rootInvocations/<inv>/workUnits/<normWU>)`. If the
//     work unit exists directly in ResultDB, return `normWU`.
//  3. If `info.ParentID == ""` (top-level AnTS invocation work unit), probe
//     `rootInvocations/<inv>/workUnits/root` and return `"root"` if found.
//  4. If `info.ResultDBWorkUnitID` identifies an enclosing task work unit in
//     ResultDB, fetch that task's `ChildWorkUnits` via `BatchGetWorkUnits` to
//     locate the matching `TF_MODULE` child work unit (and, for `TF_TEST_RUN`
//     targets, the best matching `TF_TEST_RUN` grandchild work unit).
func ResolveResultDBAntsWorkUnit(ctx context.Context, client rdbpb.ResultDBClient, invID, rawWorkUnitID string, info *AntsWorkUnitInfo) string {
	normWU := NormalizeWorkUnit(rawWorkUnitID)
	if strings.Contains(normWU, ":u-") || strings.Contains(normWU, "-attempt__") {
		return normWU
	}
	if client == nil || invID == "" {
		if info != nil && info.ResultDBWorkUnitID != "" && !strings.EqualFold(info.Type, "TF_MODULE") && !strings.EqualFold(info.Type, "TF_TEST_RUN") {
			return NormalizeWorkUnit(info.ResultDBWorkUnitID)
		}
		return normWU
	}

	normInv := NormalizeInvocation(invID)
	probeCtx := logging.SetLevel(ctx, logging.Error)

	// 1. Check if normWU exists directly in ResultDB (e.g. TFC_COMMAND_TASK).
	directName := FormatWorkUnitResourceName(normInv, normWU)
	if wu, err := client.GetWorkUnit(probeCtx, &rdbpb.GetWorkUnitRequest{
		Name: directName,
		View: rdbpb.WorkUnitView_WORK_UNIT_VIEW_BASIC,
	}); err == nil && wu != nil {
		return normWU
	}

	if info == nil {
		return normWU
	}

	// 2. Top-level AnTS invocation work units have no parent and map to "root".
	if info.ParentID == "" {
		rootName := FormatWorkUnitResourceName(normInv, "root")
		if wu, err := client.GetWorkUnit(probeCtx, &rdbpb.GetWorkUnitRequest{
			Name: rootName,
			View: rdbpb.WorkUnitView_WORK_UNIT_VIEW_BASIC,
		}); err == nil && wu != nil {
			return "root"
		}
	}

	// 3. Leaf/child AnTS work units (TF_MODULE, TF_TEST_RUN) are stored under
	// their parent task's ResultDB work unit as `<task_wu_id>:u-<uuid>`.
	parentTaskWU := NormalizeWorkUnit(info.ResultDBWorkUnitID)
	if parentTaskWU == "" {
		return normWU
	}

	taskName := FormatWorkUnitResourceName(normInv, parentTaskWU)
	taskWU, err := client.GetWorkUnit(probeCtx, &rdbpb.GetWorkUnitRequest{
		Name: taskName,
		View: rdbpb.WorkUnitView_WORK_UNIT_VIEW_BASIC,
	})
	if err != nil || taskWU == nil {
		return normWU
	}

	// If the AnTS work unit is not a module or test-run work unit and has no
	// module name, it maps to the task work unit itself.
	if !strings.EqualFold(info.Type, "TF_MODULE") && !strings.EqualFold(info.Type, "TF_TEST_RUN") && info.ModuleName == "" {
		return parentTaskWU
	}

	if len(taskWU.GetChildWorkUnits()) == 0 {
		return normWU
	}

	// BatchGet the immediate child work units of the task (typically TF_MODULE work units).
	batchResp, err := client.BatchGetWorkUnits(probeCtx, &rdbpb.BatchGetWorkUnitsRequest{
		Parent: "rootInvocations/" + normInv,
		Names:  taskWU.GetChildWorkUnits(),
		View:   rdbpb.WorkUnitView_WORK_UNIT_VIEW_FULL,
	})
	if err != nil || len(batchResp.GetWorkUnits()) == 0 {
		return normWU
	}

	matchedModuleWU := findMatchingModuleWorkUnit(batchResp.GetWorkUnits(), info)
	if matchedModuleWU == nil {
		return normWU
	}

	// If the target AnTS work unit is a TF_MODULE (or the module work unit has
	// no test-run children), return the module work unit's ResultDB ID.
	if strings.EqualFold(info.Type, "TF_MODULE") || len(matchedModuleWU.GetChildWorkUnits()) == 0 {
		return extractWorkUnitID(matchedModuleWU, normWU)
	}

	// For TF_TEST_RUN (or deeper child work units), select the best matching
	// test-run child under the matched module work unit.
	children := matchedModuleWU.GetChildWorkUnits()
	if len(children) == 1 {
		if id := extractWorkUnitIDFromName(children[0]); id != "" {
			return id
		}
	}

	if selected := selectTestRunWorkUnit(probeCtx, client, normInv, children, info); selected != "" {
		return selected
	}

	return extractWorkUnitID(matchedModuleWU, normWU)
}

// selectTestRunWorkUnit disambiguates multiple ResultDB TF_TEST_RUN child work
// units under a module work unit using a 4-tier heuristic:
//  1. Prefer a child work unit whose ResultDB `State` is `WorkUnit_FAILED`.
//  2. Query `QueryTestVerdicts` for the module with `ui_priority` ordering and
//     select the child work unit containing an unexpected or failing test result.
//  3. If the AnTS work unit includes a `CreationTimestamp`, select the child
//     work unit with the closest ResultDB `CreateTime`.
//  4. Fall back to the first child work unit.
func selectTestRunWorkUnit(ctx context.Context, client rdbpb.ResultDBClient, normInv string, childNames []string, info *AntsWorkUnitInfo) string {
	if len(childNames) == 0 {
		return ""
	}

	childBatch, err := client.BatchGetWorkUnits(ctx, &rdbpb.BatchGetWorkUnitsRequest{
		Parent: "rootInvocations/" + normInv,
		Names:  childNames,
		View:   rdbpb.WorkUnitView_WORK_UNIT_VIEW_FULL,
	})
	if err == nil && len(childBatch.GetWorkUnits()) > 0 {
		wus := childBatch.GetWorkUnits()

		// 1. Prefer a child work unit explicitly in WorkUnit_FAILED state.
		for _, cw := range wus {
			if cw.GetState() == rdbpb.WorkUnit_FAILED {
				if id := extractWorkUnitID(cw, ""); id != "" {
					return id
				}
			}
		}

		// 2. If we know the module name, check QueryTestVerdicts to find which
		// child work unit produced a failing or unexpected test result.
		if info != nil && info.ModuleName != "" {
			candidateSet := make(map[string]bool, len(childNames))
			for _, n := range childNames {
				if id := extractWorkUnitIDFromName(n); id != "" {
					candidateSet[id] = true
				}
			}

			tvResp, tvErr := client.QueryTestVerdicts(ctx, &rdbpb.QueryTestVerdictsRequest{
				Parent:   "rootInvocations/" + normInv,
				View:     rdbpb.TestVerdictView_TEST_VERDICT_VIEW_FULL,
				PageSize: 100,
				OrderBy:  "ui_priority",
				Predicate: &rdbpb.TestVerdictPredicate{
					ContainsTestResultFilter: fmt.Sprintf("test_id_structured.module_name = %q", info.ModuleName),
				},
			})
			if tvErr == nil {
				var fallbackWUID string
				for _, v := range tvResp.GetTestVerdicts() {
					for _, tr := range v.GetResults() {
						wuID := extractWorkUnitIDFromName(tr.GetName())
						if !candidateSet[wuID] {
							continue
						}
						if isFailingTestResult(tr) {
							return wuID
						}
						if fallbackWUID == "" {
							fallbackWUID = wuID
						}
					}
				}
				if fallbackWUID != "" {
					return fallbackWUID
				}
			}
		}

		// 3. Match by closest creation timestamp if available.
		if info != nil && info.CreationTimestamp > 0 {
			var bestWU *rdbpb.WorkUnit
			var bestDiff int64 = -1
			for _, cw := range wus {
				if cw.GetCreateTime() == nil {
					continue
				}
				diff := cw.GetCreateTime().AsTime().UnixMilli() - info.CreationTimestamp
				if diff < 0 {
					diff = -diff
				}
				if bestDiff < 0 || diff < bestDiff {
					bestDiff = diff
					bestWU = cw
				}
			}
			if bestWU != nil {
				if id := extractWorkUnitID(bestWU, ""); id != "" {
					return id
				}
			}
		}
	}

	// 4. Fallback to the first child work unit.
	return extractWorkUnitIDFromName(childNames[0])
}

// isFailingTestResult returns true if a ResultDB TestResult is unexpected or
// has a failure, crash, abort, precluded, or execution-errored status.
func isFailingTestResult(tr *rdbpb.TestResult) bool {
	return tr.GetStatusV2() == rdbpb.TestResult_FAILED ||
		tr.GetStatusV2() == rdbpb.TestResult_EXECUTION_ERRORED ||
		tr.GetStatusV2() == rdbpb.TestResult_PRECLUDED ||
		(!tr.GetExpected() && tr.GetStatusV2() != rdbpb.TestResult_PASSED && tr.GetStatusV2() != rdbpb.TestResult_SKIPPED) ||
		tr.GetStatus() == rdbpb.TestStatus_FAIL ||
		tr.GetStatus() == rdbpb.TestStatus_CRASH ||
		tr.GetStatus() == rdbpb.TestStatus_ABORT
}

// findMatchingModuleWorkUnit selects the ResultDB child WorkUnit whose module
// name and variant parameters (such as ABI) best match the AnTS work unit.
func findMatchingModuleWorkUnit(candidates []*rdbpb.WorkUnit, info *AntsWorkUnitInfo) *rdbpb.WorkUnit {
	if len(candidates) == 0 {
		return nil
	}

	// Filter to TF_MODULE work units (or work units with ModuleId) when present.
	var moduleWUs []*rdbpb.WorkUnit
	for _, wu := range candidates {
		if strings.EqualFold(wu.GetKind(), "TF_MODULE") || wu.GetModuleId() != nil {
			moduleWUs = append(moduleWUs, wu)
		}
	}
	if len(moduleWUs) == 0 {
		moduleWUs = candidates
	}

	if info == nil || info.ModuleName == "" {
		if len(moduleWUs) == 1 {
			return moduleWUs[0]
		}
		return nil
	}

	var nameMatches []*rdbpb.WorkUnit
	for _, wu := range moduleWUs {
		modName := wu.GetModuleId().GetModuleName()
		if modName == info.ModuleName || strings.EqualFold(modName, info.ModuleName) || strings.EqualFold(wu.GetName(), info.ModuleName) {
			nameMatches = append(nameMatches, wu)
		}
	}
	if len(nameMatches) == 0 {
		if len(moduleWUs) == 1 {
			return moduleWUs[0]
		}
		return nil
	}
	if len(nameMatches) == 1 {
		return nameMatches[0]
	}

	// Disambiguate multiple modules with the same name by matching module
	// parameters (e.g., module-abi / module_abi) or work unit name prefix.
	var wantABI string
	for _, p := range info.ModuleParameters {
		if p.Name == "module-abi" || p.Name == "module_abi" || p.Name == "abi" {
			wantABI = p.Value
			break
		}
	}
	if wantABI == "" && strings.Contains(info.Name, " ") {
		if fields := strings.Fields(info.Name); len(fields) > 0 {
			wantABI = fields[0]
		}
	}

	if wantABI != "" {
		for _, wu := range nameMatches {
			def := wu.GetModuleId().GetModuleVariant().GetDef()
			if def["module_abi"] == wantABI || def["module-abi"] == wantABI || def["abi"] == wantABI {
				return wu
			}
		}
	}

	return nameMatches[0]
}

// extractWorkUnitID returns a ResultDB WorkUnit's ID from either its WorkUnitId
// field or its resource Name, falling back to fallback if neither is set.
func extractWorkUnitID(wu *rdbpb.WorkUnit, fallback string) string {
	if wu == nil {
		return fallback
	}
	if wu.GetWorkUnitId() != "" {
		return wu.GetWorkUnitId()
	}
	if id := extractWorkUnitIDFromName(wu.GetName()); id != "" {
		return id
	}
	return fallback
}

// extractWorkUnitIDFromName extracts the work unit ID component from a
// ResultDB resource name (`rootInvocations/<inv>/workUnits/<wu>[/...]`), or
// returns name itself if it does not contain a `workUnits/` prefix.
func extractWorkUnitIDFromName(name string) string {
	if _, wuID := ExtractWorkUnitComponents(name); wuID != "" {
		return wuID
	}
	return name
}
