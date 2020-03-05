// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

func strPairs(pairs []*swarming.SwarmingRpcsStringPair) []*swarmingpb.StringPair {
	ret := make([]*swarmingpb.StringPair, len(pairs))
	for i, p := range pairs {
		ret[i] = &swarmingpb.StringPair{Key: p.Key, Value: p.Value}
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
		ContainmentType:           swarmingpb.Containment_ContainmentType(conType),
		LimitProcesses:            con.LimitProcesses,
		LimitTotalCommittedMemory: con.LimitTotalCommittedMemory,
		LowerPriority:             con.LowerPriority,
	}
}
