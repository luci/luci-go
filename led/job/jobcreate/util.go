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

package jobcreate

import (
	"fmt"

	swarming "go.chromium.org/luci/common/api/swarming/swarming/v1"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

func cipdPins(ci *swarming.SwarmingRpcsCipdInput) (ret []*swarmingpb.CIPDPackage) {
	if ci == nil {
		return
	}
	ret = make([]*swarmingpb.CIPDPackage, 0, len(ci.Packages))
	for _, pkg := range ci.Packages {
		ret = append(ret, &swarmingpb.CIPDPackage{
			PackageName: pkg.PackageName,
			Version:     pkg.Version,
			DestPath:    pkg.Path,
		})
	}
	return
}

func strPairs(pairs []*swarming.SwarmingRpcsStringPair, filter func(key string) bool) []*swarmingpb.StringPair {
	ret := make([]*swarmingpb.StringPair, 0, len(pairs))
	for _, p := range pairs {
		if filter != nil && !filter(p.Key) {
			continue
		}
		ret = append(ret, &swarmingpb.StringPair{Key: p.Key, Value: p.Value})
	}
	return ret
}

func strListPairs(pairs []*swarming.SwarmingRpcsStringListPair) []*swarmingpb.StringListPair {
	ret := make([]*swarmingpb.StringListPair, len(pairs))
	for i, p := range pairs {
		vals := make([]string, len(p.Value))
		copy(vals, p.Value)
		ret[i] = &swarmingpb.StringListPair{Key: p.Key, Values: vals}
	}
	return ret
}

// dropRecipePackage removes the all CIPDPackages in pkgs whose DestPath
// matches checkoutDir.
func dropRecipePackage(pkgs *[]*swarmingpb.CIPDPackage, checkoutDir string) {
	ret := make([]*swarmingpb.CIPDPackage, 0, len(*pkgs))
	for _, pkg := range *pkgs {
		if pkg.DestPath != checkoutDir {
			ret = append(ret, pkg)
		}
	}
	*pkgs = ret
}

func containmentFromSwarming(con *swarming.SwarmingRpcsContainment) *swarmingpb.Containment {
	if con == nil {
		return nil
	}
	conType, ok := swarmingpb.Containment_ContainmentType_value[con.ContainmentType]
	if !ok {
		// TODO(iannucci): handle this more gracefully?
		//
		// This is a relatively unused field, and I don't expect any divergence
		// between the proto / endpoints definitions...  hopefully by the time we
		// touch this swarming has a real prpc api and then this entire file can go
		// away.
		panic(fmt.Sprintf("unknown containment type %q", con.ContainmentType))
	}
	return &swarmingpb.Containment{
		ContainmentType: swarmingpb.Containment_ContainmentType(conType),
	}
}
