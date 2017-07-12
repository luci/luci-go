// Copyright 2016 The LUCI Authors.
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

package buildbucket

import (
	"bytes"
	"strconv"
)

// This file is about buildbucket build params in the format supported by
// Buildbot.

// properties is well-established build properties.
type properties struct {
	PatchStorage string `json:"patch_storage"` // e.g. "rietveld", "gerrit"

	RietveldURL string `json:"rietveld"` // e.g. "https://codereview.chromium.org"
	Issue       number `json:"issue"`    // e.g. 2127373005
	PatchSet    number `json:"patchset"` // e.g. 40001 for rietveld

	GerritPatchURL           string `json:"patch_gerrit_url"`     // e.g. "https://chromium-review.googlesource.com"
	GerritPatchIssue         int    `json:"patch_issue"`          // e.g. 358171
	GerritPatchSet           number `json:"patch_set"`            // e.g. 1
	GerritPatchProject       string `json:"patch_project"`        // e.g. "infra/infra"
	GerritPatchRepositoryURL string `json:"patch_repository_url"` // e.g. https://chromium.googlesource.com/infra/infra

	Revision  string   `json:"revision"`  // e.g. "0b04861933367c62630751702c84fd64bc3caf6f"
	BlameList []string `json:"blamelist"` // e.g. ["someone@chromium.org"]

	// Fields below are present only in ResultDetails.

	GotRevision string `json:"got_revision"` // e.g. "0b04861933367c62630751702c84fd64bc3caf6f"
	BuildNumber int    `json:"buildnumber"`  // e.g. 3021
}

// number is an integer that supports JSON unmarshalling from a string.
type number int

// UnmarshalJSON parses data as an integer, whether data is a number or string.
func (n *number) UnmarshalJSON(data []byte) error {
	data = bytes.Trim(data, `"`)
	num, err := strconv.Atoi(string(data))
	if err == nil {
		*n = number(num)
	}
	return err
}

// change is used in "changes" buildbucket parameters; supported by buildbot
// See https://chromium.googlesource.com/chromium/tools/build/+/master/scripts/master/buildbucket/README.md#Build-parameters
type change struct {
	Author struct{ Email string }
}

// buildParameters is contents of "parameters_json" buildbucket build field
// in the format supported by Buildbot, see
// // See https://chromium.googlesource.com/chromium/tools/build/+/master/scripts/master/buildbucket/README.md#Build-parameters
//
// Buildbucket is not aware of this format, but majority of chrome-infra is.
type buildParameters struct {
	BuilderName string `json:"builder_name"`
	Properties  properties
	Changes     []change
}

// resultDetails is contents of "result_details_json" buildbucket build field
// in the format supported by Buildbot.
type resultDetails struct {
	Properties properties
}
