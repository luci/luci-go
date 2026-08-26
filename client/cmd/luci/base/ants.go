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
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
)

// DefaultAntsCLIPath is the standard BinFS path to the ants_cli executable.
const DefaultAntsCLIPath = "/google/bin/releases/android/ants_cli/ants_cli"

var (
	antsTRRegex  = regexp.MustCompile(`^(?i)TR[0-9]+$`)
	antsInvRegex = regexp.MustCompile(`^(?:I[0-9]+|ants-[Ii][0-9]+)$`)
	antsWURegex  = regexp.MustCompile(`^(?:WU[0-9]+|ants-[Ww][Uu][0-9]+)$`)
)

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
	if idx := strings.IndexAny(clean, "?#"); idx != -1 {
		clean = clean[:idx]
	}

	// Match /invocation/(I[0-9]+)
	if idx := strings.Index(clean, "/invocation/"); idx != -1 {
		after := clean[idx+len("/invocation/"):]
		parts := strings.Split(after, "/")
		if len(parts) > 0 && IsAntsInvocationID(parts[0]) {
			invID = parts[0]
		}
	}

	// Match /test/(TR[0-9]+)
	if idx := strings.Index(clean, "/test/"); idx != -1 {
		after := clean[idx+len("/test/"):]
		parts := strings.Split(after, "/")
		if len(parts) > 0 && IsAntsTestResultID(parts[0]) {
			trID = parts[0]
		}
	}

	return invID, trID
}

// AntsTestResultInfo contains parsed details from an AnTS test result.
type AntsTestResultInfo struct {
	TestResultID     string `json:"test_result_id,omitempty"`
	TestCase         string `json:"test_case,omitempty"`
	Status           string `json:"status,omitempty"`
	WorkUnitID       string `json:"work_unit_id,omitempty"`
	InvocationID     string `json:"invocation_id,omitempty"`
	RunNumber        int    `json:"run_number,omitempty"`
	AttemptNumber    int    `json:"attempt_number,omitempty"`
	TestIdentifierID string `json:"test_identifier_id,omitempty"`

	// Decomposed test case components
	ModuleName    string `json:"module_name,omitempty"`
	ClassName     string `json:"class_name,omitempty"`
	MethodName    string `json:"method_name,omitempty"`
	IsModuleError bool   `json:"is_module_error,omitempty"`
}

// FindAntsCLI locates the ants_cli executable on the system.
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

// ResolveAntsTestResult executes ants_cli to retrieve details for a test result ID.
func ResolveAntsTestResult(ctx context.Context, testResultID string) (*AntsTestResultInfo, error) {
	testResultID = strings.TrimSpace(testResultID)
	if !IsAntsTestResultID(testResultID) {
		return nil, errors.Fmt("invalid AnTS test result ID %q (expected format TR<digits>)", testResultID)
	}

	if custom := os.Getenv("ANTS_CLI_PATH"); custom == "disabled" || custom == "none" {
		return nil, errors.New("AnTS test result ID resolution is disabled (ANTS_CLI_PATH=" + custom + ")")
	}

	cliPath, err := FindAntsCLI()
	if err != nil {
		return nil, err
	}

	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(callCtx, cliPath, "show-test", "-test_result_id="+testResultID)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout.String())
		}
		if errMsg != "" {
			return nil, errors.Fmt("ants_cli show-test failed: %s", errMsg)
		}
		return nil, errors.Fmt("ants_cli show-test failed: %w", err)
	}

	info, err := ParseAntsShowTestOutput(stdout.String())
	if err != nil {
		return nil, err
	}

	return info, nil
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
