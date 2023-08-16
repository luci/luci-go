// Copyright 2020 The LUCI Authors.
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

package job

import (
	"fmt"
	"strings"

	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

// CIPDPkgs is a mapping of the CIPD packages within a Definition in the form
// of:
//
//	"subdir:name/of/package" -> "version"
type CIPDPkgs map[string]string

func (c CIPDPkgs) fromList(pkgs []*swarmingpb.CipdPackage) {
	for _, pkg := range pkgs {
		if path := pkg.Path; path == "" || path == "." {
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

func (c CIPDPkgs) updateCipdPkgs(pinSets *[]*swarmingpb.CipdPackage) {
	if len(c) == 0 {
		return
	}

	// subdir -> pkg -> version
	currentMapping := map[string]map[string]string{}
	for _, pin := range *pinSets {
		subdirM, ok := currentMapping[pin.Path]
		if !ok {
			subdirM = map[string]string{}
			currentMapping[pin.Path] = subdirM
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

	newPkgs := make([]*swarmingpb.CipdPackage, 0, len(*pinSets)+len(c))
	for _, subdir := range keysOf(currentMapping) {
		for _, pkg := range keysOf(currentMapping[subdir]) {
			newPkgs = append(newPkgs, &swarmingpb.CipdPackage{
				Path:        subdir,
				PackageName: pkg,
				Version:     currentMapping[subdir][pkg],
			})
		}
	}
	*pinSets = newPkgs
}
