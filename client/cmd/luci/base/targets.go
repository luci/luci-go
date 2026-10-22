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
	"context"
	"net/url"
	"strings"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

// IsPermissionDenied reports whether err has a PermissionDenied gRPC or appstatus code.
func IsPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	code := status.Code(err)
	if code == codes.Unknown {
		code = appstatus.Code(err)
	}
	return code == codes.PermissionDenied
}

// ResolvedInvocation holds the normalized invocation ID, whether it is a legacy
// invocation, and the retrieved RootInvocation or legacy Invocation proto.
type ResolvedInvocation struct {
	ID             string
	Legacy         bool
	RootInvocation *rdbpb.RootInvocation
	Invocation     *rdbpb.Invocation
}

// ResolveInvocation normalizes rawInvID and queries ResultDB for both the
// RootInvocation and legacy Invocation in parallel to determine whether the
// target is a RootInvocation or a legacy Invocation, returning the retrieved
// invocation proto along with the resolved legacy status.
//
// If either probe returns codes.PermissionDenied, the PermissionDenied error is
// returned immediately.
func ResolveInvocation(ctx context.Context, client rdbpb.ResultDBClient, rawInvID string) (*ResolvedInvocation, error) {
	normalized := NormalizeInvocation(rawInvID)
	if client == nil || normalized == "" {
		return &ResolvedInvocation{ID: normalized}, nil
	}

	probeCtx := logging.SetLevel(ctx, logging.Error)
	var (
		rootInv   *rdbpb.RootInvocation
		rootErr   error
		legacyInv *rdbpb.Invocation
		legacyErr error
		wg        sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		rootInv, rootErr = client.GetRootInvocation(probeCtx, &rdbpb.GetRootInvocationRequest{
			Name: "rootInvocations/" + normalized,
		})
	}()
	go func() {
		defer wg.Done()
		legacyInv, legacyErr = client.GetInvocation(probeCtx, &rdbpb.GetInvocationRequest{
			Name: "invocations/" + normalized,
		})
	}()
	wg.Wait()

	if rootErr == nil {
		return &ResolvedInvocation{
			ID:             normalized,
			Legacy:         false,
			RootInvocation: rootInv,
			Invocation:     legacyInv,
		}, nil
	}
	if IsPermissionDenied(rootErr) {
		return nil, errors.Fmt("permission denied for rootInvocations/%s (missing ResultDB realm permission): %w", normalized, rootErr)
	}
	if legacyErr == nil {
		return &ResolvedInvocation{
			ID:         normalized,
			Legacy:     true,
			Invocation: legacyInv,
		}, nil
	}
	if IsPermissionDenied(legacyErr) {
		return nil, errors.Fmt("permission denied for invocations/%s (missing ResultDB realm permission): %w", normalized, legacyErr)
	}

	return &ResolvedInvocation{
		ID:     normalized,
		Legacy: false,
	}, nil
}

// NormalizeInvocation strips "rootInvocations/" or "invocations/" prefix from an invocation ID.
// If the invocation ID is an AnTS invocation (I<digits> or i<digits>), it is normalized to ants-i<digits>.
// If the invocation ID starts with a digit, it is normalized to build-<id>.
func NormalizeInvocation(inv string) string {
	inv = strings.TrimSpace(inv)
	inv = strings.TrimPrefix(inv, "invocations/")
	inv = strings.TrimPrefix(inv, "rootInvocations/")
	if strings.HasPrefix(inv, "I") && len(inv) > 1 && isAllDigits(inv[1:]) {
		return "ants-i" + inv[1:]
	}
	if strings.HasPrefix(inv, "i") && len(inv) > 1 && isAllDigits(inv[1:]) {
		return "ants-i" + inv[1:]
	}
	if strings.HasPrefix(strings.ToLower(inv), "ants-i") && len(inv) > 6 && isAllDigits(inv[6:]) {
		return "ants-i" + inv[6:]
	}
	if strings.HasPrefix(inv, "b") && len(inv) > 1 && isAllDigits(inv[1:]) {
		return "build-" + inv[1:]
	}
	if !strings.HasPrefix(inv, "build-") && !strings.HasPrefix(inv, "task-") && !strings.HasPrefix(inv, "u-") && !strings.HasPrefix(inv, "ants-") {
		if _, err := url.PathUnescape(inv); err == nil && len(inv) > 0 && (inv[0] >= '0' && inv[0] <= '9') {
			inv = "build-" + inv
		}
	}
	return inv
}

// NormalizeWorkUnit strips a leading "workUnits/" prefix from a work unit ID.
// If the ID is an AnTS work unit (e.g. WU<digits> or ants-wu<digits>), it is
// normalized to lowercase "ants-wu<digits>" while preserving any optional
// attempt (-attempt__<N>) or TradeFed composite (:u-<uuid>) suffixes.
func NormalizeWorkUnit(wu string) string {
	wu = strings.TrimPrefix(strings.TrimSpace(wu), "workUnits/")
	if m := antsWURegex.FindStringSubmatch(wu); m != nil {
		return "ants-wu" + m[1] + strings.ToLower(m[2])
	}
	return wu
}

func isAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
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

// FormatTestResultWorkUnitResourceName formats a root invocation, work unit ID, test ID, and result ID into a canonical resource name.
func FormatTestResultWorkUnitResourceName(rootInv, wuID, testID, resultID string) string {
	rootInv = NormalizeInvocation(rootInv)
	wuID = NormalizeWorkUnit(wuID)
	encodedTestID := url.PathEscape(testID)
	return "rootInvocations/" + rootInv + "/workUnits/" + wuID + "/tests/" + encodedTestID + "/results/" + resultID
}

// FormatWorkUnitResourceName formats a root invocation and work unit ID into a canonical resource name.
func FormatWorkUnitResourceName(rootInv, wuID string) string {
	rootInv = NormalizeInvocation(rootInv)
	wuID = NormalizeWorkUnit(wuID)
	return "rootInvocations/" + rootInv + "/workUnits/" + wuID
}

func unescapePathSegment(seg string) string {
	if unescaped, err := url.PathUnescape(seg); err == nil {
		return unescaped
	}
	return seg
}

// ExtractTestResultComponents parses an invocation, test ID, and result ID from a resource name.
func ExtractTestResultComponents(name string) (inv, testID, resultID string) {
	meta := ExtractArtifactMetadata(name)
	return meta.InvocationID, meta.TestID, meta.ResultID
}

// ExtractWorkUnitComponents parses a root invocation and work unit ID from a resource name.
func ExtractWorkUnitComponents(name string) (rootInv, wuID string) {
	meta := ExtractArtifactMetadata(name)
	return meta.InvocationID, meta.WorkUnitID
}

// ArtifactMetadata holds the resource metadata (invocation, work unit, test ID,
// result ID, and artifact ID) parsed from an artifact or target resource name.
type ArtifactMetadata struct {
	InvocationID string
	WorkUnitID   string
	TestID       string
	ResultID     string
	ArtifactID   string
}

// ExtractArtifactMetadata parses the owning invocation, work unit, test ID,
// result ID, and artifact ID from a resource name in a single pass.
func ExtractArtifactMetadata(name string) ArtifactMetadata {
	if idx := strings.Index(name, "rootInvocations/"); idx != -1 {
		name = name[idx:]
	} else if idx := strings.Index(name, "invocations/"); idx != -1 {
		name = name[idx:]
	}
	var meta ArtifactMetadata
	parts := strings.Split(name, "/")
	for i := 0; i+1 < len(parts); i++ {
		switch parts[i] {
		case "rootInvocations", "invocations":
			meta.InvocationID = parts[i+1]
			i++
		case "workUnits":
			meta.WorkUnitID = unescapePathSegment(parts[i+1])
			i++
		case "tests":
			meta.TestID = unescapePathSegment(parts[i+1])
			i++
		case "results":
			meta.ResultID = parts[i+1]
			i++
		case "variants":
			i++
		case "artifacts":
			meta.ArtifactID = unescapePathSegment(strings.Join(parts[i+1:], "/"))
			return meta
		}
	}
	return meta
}

// ShellQuoteIfNeeded returns s single-quoted for POSIX shells if s is empty or
// contains characters unsafe to leave unquoted, or s as-is otherwise.
func ShellQuoteIfNeeded(s string) string {
	if s == "" {
		return "''"
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.IndexByte("_@%+=:,./-", c) != -1) {
			return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
	}
	return s
}

// FormatFlags returns the copy-pasteable CLI flags (-workunitid, -testid,
// and/or -resultid) needed to target this artifact's owning resource.
func (m ArtifactMetadata) FormatFlags() string {
	var parts []string
	if m.WorkUnitID != "" {
		parts = append(parts, "-workunitid="+ShellQuoteIfNeeded(m.WorkUnitID))
	}
	if m.TestID != "" && m.ResultID != "" {
		parts = append(parts,
			"-testid="+ShellQuoteIfNeeded(m.TestID),
			"-resultid="+ShellQuoteIfNeeded(m.ResultID),
		)
	}
	return strings.Join(parts, " ")
}
