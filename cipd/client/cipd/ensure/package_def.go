// Copyright 2017 The LUCI Authors.
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

package ensure

import (
	"fmt"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"
)

// PackageDef defines a package line parsed out of an ensure file.
type PackageDef struct {
	PackageTemplate   string
	UnresolvedVersion string

	// LineNo is set while parsing an ensure file by the ParseFile method. It is
	// used by File.Resolve to give additional context if an error occurs.
	LineNo int
}

func (p PackageDef) String() string {
	return fmt.Sprintf("%s@%s", p.PackageTemplate, p.UnresolvedVersion)
}

// PackageSlice is a sortable slice of PackageDef
type PackageSlice []PackageDef

func (ps PackageSlice) Len() int           { return len(ps) }
func (ps PackageSlice) Less(i, j int) bool { return ps[i].PackageTemplate < ps[j].PackageTemplate }
func (ps PackageSlice) Swap(i, j int)      { ps[i], ps[j] = ps[j], ps[i] }

// Expand expands the package name template and checks that resulting package
// name and version are syntactically correct.
//
// May return template.ErrSkipTemplate is this package definition should be
// skipped given the current expansion variables values.
func (p *PackageDef) Expand(expander template.Expander) (pkg string, err error) {
	switch pkg, err = expander.Expand(p.PackageTemplate); {
	case err == template.ErrSkipTemplate:
		return "", err
	case err != nil:
		return "", errors.Fmt("failed to expand package template (line %d): %w", p.LineNo, err)
	}
	if err = common.ValidatePackageName(pkg); err != nil {
		return "", errors.Fmt("bad package name (line %d): %w", p.LineNo, err)
	}
	if err = common.ValidateInstanceVersion(p.UnresolvedVersion); err != nil {
		return "", errors.Fmt("bad package version (line %d): %w", p.LineNo, err)
	}
	return
}
