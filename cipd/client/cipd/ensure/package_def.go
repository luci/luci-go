// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
	return fmt.Sprintf("%s %s", p.PackageTemplate, p.UnresolvedVersion)
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
func (p *PackageDef) Resolve(plat, arch string, rslv VersionResolver) (pin common.Pin, err error) {
	pin.PackageName, err = expandTemplate(p.PackageTemplate, plat, arch)
	if err == errSkipTemplate {
		return
	}
	if err != nil {
		err = errors.Annotate(err).
			Reason("failed to resolve package template (line %(line)d)").
			D("line", p.LineNo).
			Err()
		return
	}

	pin, err = rslv(pin.PackageName, p.UnresolvedVersion)
	if err != nil {
		err = errors.Annotate(err).
			Reason("failed to resolve package version (line %(line)d)").
			D("line", p.LineNo).
			Err()
		return
	}

	return
}

// PackageSlice is a sortable slice of PackageDef
type PackageSlice []PackageDef

func (ps PackageSlice) Len() int           { return len(ps) }
func (ps PackageSlice) Less(i, j int) bool { return ps[i].PackageTemplate < ps[j].PackageTemplate }
func (ps PackageSlice) Swap(i, j int)      { ps[i], ps[j] = ps[j], ps[i] }
