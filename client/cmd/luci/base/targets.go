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
	"net/url"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// TrimResourceURL strips URL scheme/host prefix and query/hash fragments from a resource target or URL.
func TrimResourceURL(raw string) string {
	clean := strings.TrimSpace(raw)
	if idx := strings.Index(clean, "invocations/"); idx != -1 {
		clean = clean[idx:]
	} else if idx := strings.Index(clean, "rootInvocations/"); idx != -1 {
		clean = clean[idx:]
	}
	if idx := strings.IndexAny(clean, "?#"); idx != -1 {
		clean = clean[:idx]
	}
	return clean
}

// FormatTestResultResourceName formats an invocation, test ID, and result ID into a canonical resource name.
func FormatTestResultResourceName(inv, testID, resultID string) string {
	inv = NormalizeInvocation(inv)
	encodedTestID := url.PathEscape(testID)
	return "invocations/" + inv + "/tests/" + encodedTestID + "/results/" + resultID
}

// FormatWorkUnitResourceName formats a root invocation and work unit ID into a canonical resource name.
func FormatWorkUnitResourceName(rootInv, wuID string) string {
	rootInv = NormalizeInvocation(rootInv)
	return "rootInvocations/" + rootInv + "/workUnits/" + wuID
}

// ParseTestResultTargetArgs parses positional arguments for a test result target.
func ParseTestResultTargetArgs(args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("missing test result target")
	}

	cd, _ := LoadCache()

	if len(args) == 1 {
		raw := strings.TrimSpace(args[0])
		if raw == "-" {
			if cd.TestResult != "" {
				return cd.TestResult, nil
			}
			if cd.Verdict != "" {
				return cd.Verdict, nil
			}
			if cd.Invocation != "" && cd.TestID != "" && cd.ResultID != "" {
				return FormatTestResultResourceName(cd.Invocation, cd.TestID, cd.ResultID), nil
			}
			return "", errors.New("no previous test result found in cache; please specify '<inv> <test_id> <result_id>' or full resource name")
		}
		if strings.Contains(raw, "/") || strings.HasPrefix(raw, "invocations/") || strings.HasPrefix(raw, "rootInvocations/") {
			return raw, nil
		}
		return "", errors.Fmt("%q is not a full test result resource name or URL; specify '<inv> <test_id> <result_id>' or use '-' to reference cached context (e.g. 'luci test-result get - %s')", raw, raw)
	}

	if len(args) == 2 && args[0] == "-" {
		override := strings.TrimSpace(args[1])
		if cd.Verdict != "" {
			if u, err := url.Parse(cd.Verdict); err == nil {
				q := u.Query()
				q.Set("result", override)
				u.RawQuery = q.Encode()
				return u.String(), nil
			}
			return cd.Verdict + "?result=" + url.QueryEscape(override), nil
		}
		if cd.TestResult != "" {
			extInv, extTest, _ := ExtractTestResultComponents(cd.TestResult)
			if extInv != "" && extTest != "" {
				return FormatTestResultResourceName(extInv, extTest, override), nil
			}
		}
		if cd.Invocation != "" && cd.TestID != "" {
			return FormatTestResultResourceName(cd.Invocation, cd.TestID, override), nil
		}
		return "", errors.Fmt("no previous test or verdict found in cache to override result ID %q", override)
	}

	cachedInv := cd.Invocation
	cachedTestID := cd.TestID
	cachedResultID := cd.ResultID
	if cd.TestResult != "" {
		extInv, extTest, extRes := ExtractTestResultComponents(cd.TestResult)
		if extInv != "" {
			cachedInv = extInv
		}
		if extTest != "" {
			cachedTestID = extTest
		}
		if extRes != "" {
			cachedResultID = extRes
		}
	}

	fields := []string{"invocation", "test ID", "result ID"}
	cached := []string{cachedInv, cachedTestID, cachedResultID}

	resolved, direct, err := ResolveHierarchyArgs(args, fields, cached)
	if err != nil {
		return "", err
	}
	if direct != "" {
		return direct, nil
	}
	return FormatTestResultResourceName(resolved[0], resolved[1], resolved[2]), nil
}

// ParseWorkUnitTargetArgs parses positional arguments for a work unit target.
func ParseWorkUnitTargetArgs(args []string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("missing work unit target")
	}

	cd, _ := LoadCache()

	if len(args) == 1 {
		raw := strings.TrimSpace(args[0])
		if raw == "-" {
			if cd.WorkUnit != "" {
				return cd.WorkUnit, nil
			}
			if cd.Invocation != "" && cd.WorkUnitID != "" {
				return FormatWorkUnitResourceName(cd.Invocation, cd.WorkUnitID), nil
			}
			return "", errors.New("no previous work unit found in cache; please specify '<root_inv> <work_unit_id>' or full resource name")
		}
		if strings.Contains(raw, "/") || strings.HasPrefix(raw, "rootInvocations/") || strings.HasPrefix(raw, "invocations/") {
			return raw, nil
		}
		return "", errors.Fmt("%q is not a full work unit resource name or URL; specify '<root_inv> <work_unit_id>' or use '-' to reference cached context (e.g. 'luci work-unit get - %s')", raw, raw)
	}

	cachedInv := cd.Invocation
	cachedWuID := cd.WorkUnitID
	if cd.WorkUnit != "" {
		extRootInv, extWuID := ExtractWorkUnitComponents(cd.WorkUnit)
		if extRootInv != "" {
			cachedInv = extRootInv
		}
		if extWuID != "" {
			cachedWuID = extWuID
		}
	}

	fields := []string{"invocation", "work unit ID"}
	cached := []string{cachedInv, cachedWuID}

	resolved, direct, err := ResolveHierarchyArgs(args, fields, cached)
	if err != nil {
		return "", err
	}
	if direct != "" {
		return direct, nil
	}
	return FormatWorkUnitResourceName(resolved[0], resolved[1]), nil
}

// ExtractTestResultComponents parses an invocation, test ID, and result ID from a resource name.
func ExtractTestResultComponents(name string) (inv, testID, resultID string) {
	if idx := strings.Index(name, "invocations/"); idx != -1 {
		name = name[idx:]
	}
	if idx := strings.Index(name, "/artifacts/"); idx != -1 {
		name = name[:idx]
	}
	parts := strings.Split(name, "/")
	for i := 0; i < len(parts); i++ {
		if parts[i] == "invocations" && i+1 < len(parts) {
			inv = parts[i+1]
		}
		if parts[i] == "tests" && i+1 < len(parts) {
			if unescaped, err := url.PathUnescape(parts[i+1]); err == nil {
				testID = unescaped
			} else {
				testID = parts[i+1]
			}
		}
		if parts[i] == "results" && i+1 < len(parts) {
			resultID = parts[i+1]
		}
	}
	return inv, testID, resultID
}

// ExtractWorkUnitComponents parses a root invocation and work unit ID from a resource name.
func ExtractWorkUnitComponents(name string) (rootInv, wuID string) {
	if idx := strings.Index(name, "rootInvocations/"); idx != -1 {
		name = name[idx:]
	} else if idx := strings.Index(name, "invocations/"); idx != -1 {
		name = name[idx:]
	}
	if idx := strings.Index(name, "/artifacts/"); idx != -1 {
		name = name[:idx]
	}
	parts := strings.Split(name, "/")
	for i := 0; i < len(parts); i++ {
		if (parts[i] == "rootInvocations" || parts[i] == "invocations") && i+1 < len(parts) {
			rootInv = parts[i+1]
		}
		if parts[i] == "workUnits" && i+1 < len(parts) {
			wuID = parts[i+1]
		}
	}
	return rootInv, wuID
}
