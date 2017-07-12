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

	"github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/common/errors"
)

// PackageDef defines a package line parsed out of an ensure file.
type PackageDef struct {
	PackageTemplate   string
	UnresolvedVersion string

	// LineNo is set while parsing an ensure file by the ParseFile method. It is
	// used by File.Resolve to give additional context if an error occurs.
	LineNo int
}

func (p *PackageDef) String() string {
	return fmt.Sprintf("%s@%s", p.PackageTemplate, p.UnresolvedVersion)
}

// VersionResolver is expected to tranform a {PackageName, Version} tuple into
// a resolved instance ID.
//
//  - `pkg` is guaranteed to pass common.ValidatePackageName
//  - `vers` is guaranteed to pass common.ValidateInstanceVersion
//
// The returned `instID` will NOT be checked (so that you
// can provide a pass-through resolver, if you like).
type VersionResolver func(pkg, vers string) (common.Pin, error)

// Resolve takes a Package definition containing a possibly templated package
// name, and a possibly unresolved version string and attempts to resolve them
// into a Pin.
//
// templateArgs is a mapping of expansion parameter to value. Usually you'll
// want to pass common.TemplateArgs().
func (p *PackageDef) Resolve(rslv VersionResolver, templateArgs map[string]string) (pin common.Pin, err error) {
	pin.PackageName, err = expandTemplate(p.PackageTemplate, templateArgs)
	if err == errSkipTemplate {
		return
	}
	if err != nil {
		err = errors.Annotate(err, "failed to resolve package template (line %d)", p.LineNo).Err()
		return
	}

	pin, err = rslv(pin.PackageName, p.UnresolvedVersion)
	if err != nil {
		err = errors.Annotate(err, "failed to resolve package version (line %d)", p.LineNo).Err()
		return
	}

	return
}

// PackageSlice is a sortable slice of PackageDef
type PackageSlice []PackageDef

func (ps PackageSlice) Len() int           { return len(ps) }
func (ps PackageSlice) Less(i, j int) bool { return ps[i].PackageTemplate < ps[j].PackageTemplate }
func (ps PackageSlice) Swap(i, j int)      { ps[i], ps[j] = ps[j], ps[i] }
