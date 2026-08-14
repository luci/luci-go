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

// Package base provides common utilities, context caching, argument resolution,
// and authentication helpers for the LUCI CLI.
//
// Context caching persists recently accessed resources (such as invocation ID,
// work unit ID, test ID, result ID, and verdict URL) to disk in the user's cache
// directory ($XDG_CACHE_HOME/luci, ~/.cache/luci, or ~/.luci). This enables
// users to reference previously fetched resources using '-' and supply trailing
// override arguments without retyping long resource paths.
// When a parent resource (e.g. Invocation) is updated, all cached child resources
// (e.g. WorkUnit, TestID, ResultID, Artifact) are automatically invalidated.

package base

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.chromium.org/luci/common/errors"
)

// CacheData stores the most recently accessed LUCI CLI resource components.
type CacheData struct {
	Invocation string `json:"invocation,omitempty"`
	WorkUnitID string `json:"work_unit_id,omitempty"`
	WorkUnit   string `json:"work_unit,omitempty"`
	TestID     string `json:"test_id,omitempty"`
	ResultID   string `json:"result_id,omitempty"`
	TestResult string `json:"test_result,omitempty"`
	Verdict    string `json:"verdict,omitempty"`
	Artifact   string `json:"artifact,omitempty"`
}

var (
	cacheMu      sync.Mutex
	testCacheDir string
)

func getCachePath() string {
	if testCacheDir != "" {
		return filepath.Join(testCacheDir, "last_resources.json")
	}
	if cacheDir, err := os.UserCacheDir(); err == nil && cacheDir != "" {
		return filepath.Join(cacheDir, "luci", "last_resources.json")
	}
	if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
		return filepath.Join(homeDir, ".luci", "last_resources.json")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("luci_last_resources_%d.json", os.Getuid()))
}

// SetTestCacheDir sets a temporary cache directory for testing.
func SetTestCacheDir(dir string) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	testCacheDir = dir
}

// LoadCache loads the cached resources.
func LoadCache() (*CacheData, error) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	p := getCachePath()
	data, err := os.ReadFile(p)
	if err != nil {
		return &CacheData{}, nil
	}
	var cd CacheData
	if err := json.Unmarshal(data, &cd); err != nil {
		return &CacheData{}, nil
	}
	return &cd, nil
}

// SaveCache updates and persists the cached resources.
func SaveCache(update func(cd *CacheData)) error {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	p := getCachePath()
	var cd CacheData
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &cd)
	}
	update(&cd)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(&cd, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

func isFlagLike(s string) bool {
	return strings.HasPrefix(s, "--") || (strings.HasPrefix(s, "-") && len(s) > 1 && (s[1] >= 'a' && s[1] <= 'z' || s[1] >= 'A' && s[1] <= 'Z'))
}

// ResolveHierarchyArgs resolves positional arguments against a hierarchical resource scheme.
// If a single non-dash argument is passed, it is returned directly as a raw name/URL.
// If '-' is passed, trailing arguments override components from the end.
func ResolveHierarchyArgs(args []string, fieldNames []string, cachedValues []string) ([]string, string, error) {
	if len(args) == 0 {
		return nil, "", errors.New("missing target argument")
	}
	for _, arg := range args {
		if isFlagLike(arg) {
			return nil, "", errors.Fmt("flags must be placed before positional arguments: found %q", arg)
		}
	}

	n := len(fieldNames)
	if len(args) == 1 && args[0] != "-" {
		return nil, strings.TrimSpace(args[0]), nil
	}

	if args[0] == "-" {
		overrides := len(args) - 1
		if overrides >= n {
			return nil, "", errors.Fmt("too many arguments: expected at most %d components", n)
		}
		cachedCount := n - overrides
		result := make([]string, n)
		for i := 0; i < cachedCount; i++ {
			val := strings.TrimSpace(cachedValues[i])
			if val == "" {
				return nil, "", errors.Fmt("no previous %s found in cache; please specify a %s", fieldNames[i], fieldNames[i])
			}
			result[i] = val
		}
		for i := 0; i < overrides; i++ {
			result[cachedCount+i] = strings.TrimSpace(args[1+i])
		}
		return result, "", nil
	}

	if len(args) == n {
		result := make([]string, n)
		for i := 0; i < n; i++ {
			result[i] = strings.TrimSpace(args[i])
		}
		return result, "", nil
	}

	return nil, "", errors.Fmt("invalid arguments: expected <name>, '-' with up to %d overrides, or %d decomposed IDs", n-1, n)
}

// NormalizeInvocation normalizes an invocation identifier by stripping prefix.
func NormalizeInvocation(inv string) string {
	inv = strings.TrimSpace(inv)
	inv = strings.TrimPrefix(inv, "invocations/")
	inv = strings.TrimPrefix(inv, "rootInvocations/")
	if !strings.HasPrefix(inv, "build-") && !strings.HasPrefix(inv, "task-") && !strings.HasPrefix(inv, "u-") {
		if _, err := url.PathUnescape(inv); err == nil && len(inv) > 0 && (inv[0] >= '0' && inv[0] <= '9') {
			inv = "build-" + inv
		}
	}
	return inv
}

// SetInvocation updates the cached invocation, invalidating child resources if changed.
func (cd *CacheData) SetInvocation(inv string) {
	inv = NormalizeInvocation(inv)
	if inv == "" || inv == "-" || isFlagLike(inv) {
		return
	}
	if cd.Invocation != inv {
		cd.Invocation = inv
		// Invalidate all children of the old invocation
		cd.WorkUnitID = ""
		cd.WorkUnit = ""
		cd.TestID = ""
		cd.ResultID = ""
		cd.TestResult = ""
		cd.Verdict = ""
		cd.Artifact = ""
	}
}

// SetTestID updates the cached test ID, invalidating child results if changed.
func (cd *CacheData) SetTestID(testID string) {
	testID = strings.TrimSpace(testID)
	if testID == "" || testID == "-" || isFlagLike(testID) {
		return
	}
	if cd.TestID != testID {
		cd.TestID = testID
		cd.ResultID = ""
		cd.TestResult = ""
		cd.Artifact = ""
	}
}

// SetTestResult updates the cached test result, invocation, and test ID.
func (cd *CacheData) SetTestResult(inv, testID, resultID, fullName string) {
	if inv != "" && inv != "-" {
		cd.SetInvocation(inv)
	}
	if testID != "" && testID != "-" {
		cd.SetTestID(testID)
	}
	resultID = strings.TrimSpace(resultID)
	if resultID != "" && resultID != "-" && !isFlagLike(resultID) {
		if cd.ResultID != resultID {
			cd.ResultID = resultID
			cd.Artifact = ""
		}
	}
	if fullName != "" && fullName != "-" && !isFlagLike(fullName) {
		if idx := strings.Index(fullName, "/artifacts/"); idx != -1 {
			fullName = fullName[:idx]
		}
		cd.TestResult = fullName
	}
}

// SetWorkUnit updates the cached work unit and root invocation.
func (cd *CacheData) SetWorkUnit(rootInv, wuID, fullName string) {
	if rootInv != "" && rootInv != "-" {
		cd.SetInvocation(rootInv)
	}
	wuID = strings.TrimSpace(wuID)
	if wuID != "" && wuID != "-" && !isFlagLike(wuID) {
		if cd.WorkUnitID != wuID {
			cd.WorkUnitID = wuID
			cd.Artifact = ""
		}
	}
	if fullName != "" && fullName != "-" && !isFlagLike(fullName) {
		if idx := strings.Index(fullName, "/artifacts/"); idx != -1 {
			fullName = fullName[:idx]
		}
		cd.WorkUnit = fullName
	}
}

// SetVerdict updates the cached verdict and invocation.
func (cd *CacheData) SetVerdict(inv, verdictURL string) {
	if inv != "" && inv != "-" {
		cd.SetInvocation(inv)
	}
	verdictURL = strings.TrimSpace(verdictURL)
	if verdictURL != "" && verdictURL != "-" && !isFlagLike(verdictURL) {
		cd.Verdict = verdictURL
	}
}

// RecordVerdict records a user-provided verdict and invocation.
func RecordVerdict(inv, verdictURL string) {
	_ = SaveCache(func(cd *CacheData) {
		cd.SetVerdict(inv, verdictURL)
	})
}

// SetArtifact updates the cached artifact ID.
func (cd *CacheData) SetArtifact(artID string) {
	artID = strings.TrimSpace(artID)
	if artID == "" || artID == "-" || isFlagLike(artID) {
		return
	}
	cd.Artifact = artID
}

// RecordTestResult records a test result, extracting and setting components.
func RecordTestResult(resName string, inv, testID string) {
	_ = SaveCache(func(cd *CacheData) {
		extractedInv, extractedTestID, extractedResultID := ExtractTestResultComponents(resName)
		if inv == "" {
			inv = extractedInv
		}
		if testID == "" {
			testID = extractedTestID
		}
		cd.SetTestResult(inv, testID, extractedResultID, resName)
	})
}

// RecordTestResultArtifact records a test result and artifact atomically.
func RecordTestResultArtifact(resName string, inv, testID, resultID, artID string) {
	_ = SaveCache(func(cd *CacheData) {
		extractedInv, extractedTestID, extractedResultID := ExtractTestResultComponents(resName)
		if inv == "" {
			inv = extractedInv
		}
		if testID == "" {
			testID = extractedTestID
		}
		if resultID == "" {
			resultID = extractedResultID
		}
		cd.SetTestResult(inv, testID, resultID, resName)
		cd.SetArtifact(artID)
	})
}

// RecordWorkUnit records a work unit, extracting and setting components.
func RecordWorkUnit(wuName string, rootInv string) {
	_ = SaveCache(func(cd *CacheData) {
		extractedRootInv, extractedWuID := ExtractWorkUnitComponents(wuName)
		if rootInv == "" {
			rootInv = extractedRootInv
		}
		cd.SetWorkUnit(rootInv, extractedWuID, wuName)
	})
}

// RecordWorkUnitArtifact records a work unit and artifact atomically.
func RecordWorkUnitArtifact(wuName string, rootInv, wuID, artID string) {
	_ = SaveCache(func(cd *CacheData) {
		extractedRootInv, extractedWuID := ExtractWorkUnitComponents(wuName)
		if rootInv == "" {
			rootInv = extractedRootInv
		}
		if wuID == "" {
			wuID = extractedWuID
		}
		cd.SetWorkUnit(rootInv, wuID, wuName)
		cd.SetArtifact(artID)
	})
}

// RecordArtifact records an artifact ID.
func RecordArtifact(artID string) {
	_ = SaveCache(func(cd *CacheData) {
		cd.SetArtifact(artID)
	})
}
