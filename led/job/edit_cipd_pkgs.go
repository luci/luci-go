// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
	fmt "fmt"
	"strings"

	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

// CIPDPkgs is a mapping of the CIPD packages within a Definition in the form
// of:
//
//    "subdir:name/of/package" -> "version"
type CIPDPkgs map[string]string

func (c CIPDPkgs) fromList(pkgs []*swarmingpb.CIPDPackage) {
	for _, pkg := range pkgs {
		if path := pkg.DestPath; path == "" || path == "." {
			c[pkg.PackageName] = pkg.Version
		} else {
			c[fmt.Sprintf("%s:%s", path, pkg.PackageName)] = pkg.Version
		}
	}
}

func (c CIPDPkgs) equal(other CIPDPkgs) bool {
	if len(c) != len(other) {
		return false
	}
	for key := range c {
		if c[key] != other[key] {
			return false
		}
	}
	return true
}

func (c CIPDPkgs) updateCipdPkgs(pinSets *[]*swarmingpb.CIPDPackage) {
	if len(c) == 0 {
		return
	}

	// subdir -> pkg -> version
	currentMapping := map[string]map[string]string{}
	for _, pin := range *pinSets {
		subdirM, ok := currentMapping[pin.DestPath]
		if !ok {
			subdirM = map[string]string{}
			currentMapping[pin.DestPath] = subdirM
		}
		subdirM[pin.PackageName] = pin.Version
	}

	for subdirPkg, vers := range c {
		subdir := "."
		pkg := subdirPkg
		if toks := strings.SplitN(subdirPkg, ":", 2); len(toks) > 1 {
			subdir, pkg = toks[0], toks[1]
			if subdir == "" {
				subdir = "."
			}
		}
		if vers == "" {
			delete(currentMapping[subdir], pkg)
		} else {
			subdirM, ok := currentMapping[subdir]
			if !ok {
				subdirM = map[string]string{}
				currentMapping[subdir] = subdirM
			}
			subdirM[pkg] = vers
		}
	}

	newPkgs := make([]*swarmingpb.CIPDPackage, 0, len(*pinSets)+len(c))
	for _, subdir := range keysOf(currentMapping) {
		for _, pkg := range keysOf(currentMapping[subdir]) {
			newPkgs = append(newPkgs, &swarmingpb.CIPDPackage{
				DestPath:    subdir,
				PackageName: pkg,
				Version:     currentMapping[subdir][pkg],
			})
		}
	}
	*pinSets = newPkgs
}
