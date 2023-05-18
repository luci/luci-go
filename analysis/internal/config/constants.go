// Copyright 2022 The LUCI Authors.
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

package config

import (
	"regexp"
)

// ProjectRePattern is the regular expression pattern that matches
// validly formed LUCI Project names.
// From https://source.chromium.org/chromium/infra/infra/+/main:luci/appengine/components/components/config/common.py?q=PROJECT_ID_PATTERN
const ProjectRePattern = `[a-z0-9\-]{1,40}`

// ProjectRe matches validly formed LUCI Project names.
var ProjectRe = regexp.MustCompile(`^` + ProjectRePattern + `$`)

// TestIDRePattern is the regular expression pattern that matches
// validly formed TestID.
const TestIDRePattern = `[[:print:]]{1,512}`

// VariantHashRePattern is the regular expression pattern that matches
// validly formed Variant Hash.
const VariantHashRePattern = `[0-9a-f]{16}`

// RefHashRePattern is the regular expression pattern that matches
// validly formed Ref Hash.
const RefHashRePattern = `[0-9a-f]{16}`
