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
