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
	"regexp"
	"strconv"
	"strings"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

const (
	// DefaultGerritHost is the default Gerrit host used when only a change ID is provided.
	DefaultGerritHost = "chromium-review.googlesource.com"
	// InternalGerritHost is the Gerrit host for Chrome internal reviews (crrev.com/i/...).
	InternalGerritHost = "chrome-internal-review.googlesource.com"
	// AndroidGerritHost is the Gerrit host for Google internal Android reviews (ag/...).
	AndroidGerritHost = "googleplex-android-review.googlesource.com"
	// AOSPGerritHost is the Gerrit host for public Android Open Source Project reviews (aosp/...).
	AOSPGerritHost = "android-review.googlesource.com"
)

var (
	// regexGerritURL matches full Gerrit review URLs on *.googlesource.com or *.git.corp.google.com.
	// Examples:
	//   https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529/10
	//   https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/8565529
	//   https://chromium-review.googlesource.com/c/8565529/1..3
	//   https://chromium-review.git.corp.google.com/c/infra/luci/luci-go/+/8565529
	//   chrome-internal-review.googlesource.com/c/chrome/+/12345/2
	regexGerritURL = regexp.MustCompile(`^(?:https?://)?([a-zA-Z0-9.-]+\.(?:googlesource|git\.corp\.google)\.com)/(?:#/)?c/(?:(.+?)/\+/)?(\d+)(?:/(?:\d+\.\.)?(\d+))?(?:/.*)?$`)

	// regexCRRev matches crrev.com shorthands.
	// Examples:
	//   https://crrev.com/c/8565529/3
	//   https://crrev.com/c/8565529/2..5
	//   crrev.com/i/12345
	regexCRRev = regexp.MustCompile(`^(?:https?://)?crrev\.com/([ci])/(\d+)(?:/(?:\d+\.\.)?(\d+))?(?:/.*)?$`)

	// regexShorthand matches c/123[/4], i/123[/4], ag/123[/4], or aosp/123[/4].
	regexShorthand = regexp.MustCompile(`^(?:https?://)?(c|i|ag|aosp)/(\d+)(?:/(?:\d+\.\.)?(\d+))?(?:/.*)?$`)

	// regexDigits matches a bare numeric change ID.
	regexDigits = regexp.MustCompile(`^\d+$`)
)

// cleanGerritTarget normalizes legacy "/#/c/" to "/c/" and strips URL query
// parameters and fragments while trimming whitespace.
func cleanGerritTarget(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.Replace(s, "/#/c/", "/c/", 1)
	if idx := strings.IndexAny(s, "?#"); idx != -1 {
		s = s[:idx]
	}
	return s
}

// IsGerritURL returns true if s is a Gerrit CL URL or shorthand (crrev.com, c/<id>, i/<id>, ag/<id>, aosp/<id>).
// It does NOT match bare numbers (to avoid colliding with bare Build IDs in 'luci ids').
func IsGerritURL(s string) bool {
	clean := cleanGerritTarget(s)
	if clean == "" {
		return false
	}
	return regexCRRev.MatchString(clean) || regexShorthand.MatchString(clean) || regexGerritURL.MatchString(clean)
}

// ParseGerritCL parses a Gerrit CL URL, crrev.com link, shorthand (c/<id>, i/<id>, ag/<id>, aosp/<id>),
// or bare change ID into a GerritChange message.
// If the patchset is not specified, Patchset is set to 0.
func ParseGerritCL(s string) (*pb.GerritChange, error) {
	clean := cleanGerritTarget(s)
	if clean == "" {
		return nil, errors.New("empty Gerrit CL target")
	}

	ret := &pb.GerritChange{}
	var changeStr, patchSetStr string

	if m := regexCRRev.FindStringSubmatch(clean); m != nil {
		if m[1] == "i" {
			ret.Host = InternalGerritHost
		} else {
			ret.Host = DefaultGerritHost
		}
		changeStr = m[2]
		patchSetStr = m[3]
	} else if m := regexShorthand.FindStringSubmatch(clean); m != nil {
		switch m[1] {
		case "i":
			ret.Host = InternalGerritHost
		case "ag":
			ret.Host = AndroidGerritHost
		case "aosp":
			ret.Host = AOSPGerritHost
		default:
			ret.Host = DefaultGerritHost
		}
		changeStr = m[2]
		patchSetStr = m[3]
	} else if m := regexGerritURL.FindStringSubmatch(clean); m != nil {
		host := m[1]
		if strings.HasSuffix(host, ".git.corp.google.com") {
			host = strings.TrimSuffix(host, ".git.corp.google.com") + ".googlesource.com"
		}
		ret.Host = host
		ret.Project = m[2]
		changeStr = m[3]
		patchSetStr = m[4]
	} else if regexDigits.MatchString(clean) {
		ret.Host = DefaultGerritHost
		changeStr = clean
	} else {
		return nil, errors.Fmt("cannot parse %q as a Gerrit CL URL or change ID", s)
	}

	changeNum, err := strconv.ParseInt(changeStr, 10, 64)
	if err != nil || changeNum <= 0 {
		return nil, errors.Fmt("invalid Gerrit change number %q", changeStr)
	}
	ret.Change = changeNum

	if patchSetStr != "" {
		psNum, err := strconv.ParseInt(patchSetStr, 10, 64)
		if err != nil || psNum <= 0 {
			return nil, errors.Fmt("invalid Gerrit patchset number %q", patchSetStr)
		}
		ret.Patchset = psNum
	}

	return ret, nil
}

// ResolveGerritPatchset resolves the latest patchset for a GerritChange if Patchset == 0.
// If change already has Patchset > 0, it is returned as is without making any RPCs.
// Otherwise, it queries Gerrit using client to find the current revision/patchset.
func ResolveGerritPatchset(ctx context.Context, client gerritpb.GerritClient, change *pb.GerritChange) (*pb.GerritChange, error) {
	if change == nil {
		return nil, errors.New("nil GerritChange")
	}
	if change.Patchset > 0 {
		return change, nil
	}
	if client == nil {
		return nil, errors.Fmt("patchset is required for CL %d (specify -patchset <n>)", change.Change)
	}

	info, err := client.GetChange(ctx, &gerritpb.GetChangeRequest{
		Number:  change.Change,
		Project: change.Project,
		Options: []gerritpb.QueryOption{gerritpb.QueryOption_CURRENT_REVISION},
	})
	if err != nil {
		return nil, errors.Fmt("failed to fetch CL %d from %s: %w", change.Change, change.Host, err)
	}

	if change.Project == "" && info.Project != "" {
		change.Project = info.Project
	}

	if rev, ok := info.Revisions[info.CurrentRevision]; ok && rev.Number > 0 {
		change.Patchset = int64(rev.Number)
		return change, nil
	}

	// Fallback to max revision number in Revisions
	var maxRev int64
	for _, rev := range info.Revisions {
		if int64(rev.Number) > maxRev {
			maxRev = int64(rev.Number)
		}
	}
	if maxRev > 0 {
		change.Patchset = maxRev
		return change, nil
	}

	return nil, errors.Fmt("could not determine latest patchset for CL %d from Gerrit", change.Change)
}
