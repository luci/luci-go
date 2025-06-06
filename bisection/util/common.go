// Copyright 2023 The LUCI Authors.
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

// Package util contains utility functions
package util

import (
	"regexp"

	"go.chromium.org/luci/common/errors"
)

// ProjectRePattern is the regular expression pattern that matches
// validly formed LUCI Project names.
// From https://source.chromium.org/chromium/infra/infra/+/main:luci/appengine/components/components/config/common.py?q=PROJECT_ID_PATTERN
const ProjectRePattern = `[a-z0-9\-]{1,40}`

// VariantHashRePattern is the regular expression pattern that matches
// validly formed variant hash.
// From https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/internal/config/constants.go;l=23
const VariantHashRePattern = `[0-9a-f]{16}`

// RefHashRePattern is the regular expression pattern that matches
// validly formed ref hash.
// From https://source.chromium.org/chromium/infra/infra/+/main:go/src/go.chromium.org/luci/analysis/internal/config/constants.go;l=27
const RefHashRePattern = `[0-9a-f]{16}`

// projectRe matches validly formed LUCI Project names.
var projectRe = regexp.MustCompile(`^` + ProjectRePattern + `$`)
var variantHashRe = regexp.MustCompile(`^` + VariantHashRePattern + `$`)
var refHashRe = regexp.MustCompile(`^` + RefHashRePattern + `$`)

func ValidateProject(project string) error {
	if project == "" {
		return errors.New("unspecified")
	}
	if !projectRe.MatchString(project) {
		return errors.Fmt("project %s must match %s", project, projectRe)
	}
	return nil
}

func ValidateVariantHash(variantHash string) error {
	if !variantHashRe.MatchString(variantHash) {
		return errors.Fmt("variant hash %s must match %s", variantHash, variantHashRe)
	}
	return nil
}

func ValidateRefHash(refHash string) error {
	if !refHashRe.MatchString(refHash) {
		return errors.Fmt("ref hash %s must match %s", refHash, refHashRe)
	}
	return nil
}
