// Copyright 2019 The LUCI Authors.
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

package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

var regexCL = regexp.MustCompile(`(\w+-review\.googlesource\.com)/(#/)?c/(([^\+]+)/\+/)?(\d+)(/(\d+))?`)

// parseCL tries to retrieve a CL info from a string.
//
// It is not strict and can consume noisy strings, e.g.
// https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7/buildbucket/cmd/bb/base_command.go
// or incomplete strings, e.g.
// https://chromium-review.googlesource.com/c/1541677
// If err is nil, returned change is guaranteed to have Host and Change.
func parseCL(s string) (*buildbucketpb.GerritChange, error) {
	m := regexCL.FindStringSubmatch(s)
	if m == nil {
		return nil, fmt.Errorf("does not match regexp %q", regexCL)
	}
	ret := &buildbucketpb.GerritChange{
		Host:    m[1],
		Project: m[4],
	}
	change := m[5]
	patchSet := m[7]

	var err error
	ret.Change, err = strconv.ParseInt(change, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid change %q: %s", change, err)
	}

	if patchSet != "" {
		ret.Patchset, err = strconv.ParseInt(patchSet, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid patchset %q: %s", patchSet, err)
		}
	}

	return ret, nil
}

var regexCommit = regexp.MustCompile(`(\w+\.googlesource\.com)/([^\+]+)/\+/(([a-f0-9]{40})|([\w\-/]+))`)

// parseCommit tries to retrieve a Gitiles Commit from a string.
//
// It is not strict or precise, and can consume noisy strings, e.g.
// https://chromium.googlesource.com/chromium/src/+/34e1ee99cc34fa86a1c2699977e223a13eb5f96c/chrome/test/chromedriver/test/run_py_tests.py
// If err is nil, returned commit is guaranteed to have Host, Project and
// either Ref or Id.
//
// If mustConfirmRef is true, commit.Ref needs to be confirmed.
func parseCommit(s string) (commit *buildbucketpb.GitilesCommit, mustConfirmRef bool, err error) {
	m := regexCommit.FindStringSubmatch(s)
	if m == nil {
		return nil, false, fmt.Errorf("does not match regexp %q", regexCommit)
	}
	commit = &buildbucketpb.GitilesCommit{
		Host:    m[1],
		Project: m[2],
		Id:      m[4],
		Ref:     m[5],
	}

	ref := strings.Split(commit.Ref, "/")
	// Apply heuristic.
	switch {
	case commit.Id != "":
		// This is a precise commit.

	case strings.HasPrefix(commit.Ref, "refs/heads/"):
		// Take one component after refs/heads/
		commit.Ref = strings.Join(ref[:3], "/")
		mustConfirmRef = len(ref) > 3

	case !strings.HasPrefix(commit.Ref, "refs/"):
		// Suspect a branch.
		commit.Ref = "refs/heads/" + ref[0]
		mustConfirmRef = true

	default:
		// The user has specified something with prefix "refs/", but without
		// prefix "refs/heads/". They must know what they is doing.
		mustConfirmRef = true

	}
	return
}
