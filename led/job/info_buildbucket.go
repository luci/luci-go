// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
	"path"

	"github.com/golang/protobuf/jsonpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

type bbInfo struct {
	*Buildbucket

	userPayload *swarmingpb.CASTree
}

var _ Info = bbInfo{}

func (b bbInfo) SwarmingHostname() string {
	return b.GetBbagentArgs().GetBuild().GetInfra().GetSwarming().GetHostname()
}

func (b bbInfo) TaskName() string {
	return b.GetName()
}

func (b bbInfo) CurrentIsolated() (*swarmingpb.CASTree, error) {
	return b.userPayload, nil
}

func (b bbInfo) Dimensions() (ExpiringDimensions, error) {
	ldims := logicalDimensions{}
	for _, reqDim := range b.BbagentArgs.Build.Infra.Swarming.TaskDimensions {
		exp := reqDim.Expiration
		if exp == nil {
			exp = b.BbagentArgs.Build.SchedulingTimeout
		}
		ldims.update(reqDim.Key, reqDim.Value, exp)
	}
	return ldims.toExpiringDimensions(), nil
}

func (b bbInfo) CIPDPkgs() (ret CIPDPkgs, err error) {
	ret = CIPDPkgs{}
	ret.fromList(b.CipdPackages)
	return
}

func (b bbInfo) Env() (ret map[string]string, err error) {
	ret = make(map[string]string, len(b.EnvVars))
	for _, pair := range b.EnvVars {
		ret[pair.Key] = pair.Value
	}
	return
}

func (b bbInfo) Priority() int32 {
	return b.GetBbagentArgs().GetBuild().GetInfra().GetSwarming().GetPriority()
}

func (b bbInfo) PrefixPathEnv() (ret []string, err error) {
	for _, keyVals := range b.EnvPrefixes {
		if keyVals.Key == "PATH" {
			ret = make([]string, len(keyVals.Values))
			copy(ret, keyVals.Values)
			break
		}
	}
	return
}

func (b bbInfo) Tags() (ret []string) {
	if len(b.ExtraTags) > 0 {
		ret = make([]string, len(b.ExtraTags))
		copy(ret, b.ExtraTags)
	}
	return
}

func (b bbInfo) Experimental() bool {
	return b.GetBbagentArgs().GetBuild().GetInput().GetExperimental()
}

func (b bbInfo) Properties() (ret map[string]string, err error) {
	if p := b.GetBbagentArgs().GetBuild().GetInput().GetProperties(); p != nil {
		m := (&jsonpb.Marshaler{})
		ret = make(map[string]string, len(p.Fields))
		for key, field := range p.Fields {
			if ret[key], err = m.MarshalToString(field); err != nil {
				ret = nil
				return
			}
		}
	}
	return
}

func (b bbInfo) GerritChanges() (ret []*bbpb.GerritChange) {
	if changes := b.GetBbagentArgs().GetBuild().GetInput().GetGerritChanges(); len(changes) > 0 {
		ret = make([]*bbpb.GerritChange, len(changes))
		for i, change := range changes {
			toAdd := *change
			ret[i] = &toAdd
		}
	}
	return
}

func (b bbInfo) GitilesCommit() (ret *bbpb.GitilesCommit) {
	if gc := b.GetBbagentArgs().GetBuild().GetInput().GetGitilesCommit(); gc != nil {
		toRet := *gc
		ret = &toRet
	}
	return
}

func (b bbInfo) TaskPayload() (cipdPkg, cipdVers string, pathInTask string) {
	exe := b.GetBbagentArgs().GetBuild().GetExe()
	cipdPkg = exe.GetCipdPackage()
	cipdVers = exe.GetCipdVersion()
	pathInTask = path.Dir(b.GetBbagentArgs().GetExecutablePath())
	return
}
