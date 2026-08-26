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
)

// NormalizeInvocation strips "rootInvocations/" or "invocations/" prefix from an invocation ID.
// If the invocation ID starts with a digit, it is normalized to build-<id>.
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

// NormalizeWorkUnit strips "workUnits/" prefix from a work unit ID.
func NormalizeWorkUnit(wu string) string {
	wu = strings.TrimSpace(wu)
	wu = strings.TrimPrefix(wu, "workUnits/")
	return wu
}

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
	wuID = NormalizeWorkUnit(wuID)
	return "rootInvocations/" + rootInv + "/workUnits/" + wuID
}

// ExtractTestResultComponents parses an invocation, test ID, and result ID from a resource name.
func ExtractTestResultComponents(name string) (inv, testID, resultID string) {
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
		if (parts[i] == "invocations" || parts[i] == "rootInvocations") && i+1 < len(parts) {
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
