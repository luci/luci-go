// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
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
	panic("implement me")
}

func (b bbInfo) Tags() (ret []string) {
	panic("implement me")
}

func (b bbInfo) Experimental() bool {
	return b.GetBbagentArgs().GetBuild().GetInput().GetExperimental()
}

func (b bbInfo) Properties() (ret map[string]string, err error) {
	panic("implement me")
}

func (b bbInfo) GerritChanges() (ret []*bbpb.GerritChange) {
	panic("implement me")
}

func (b bbInfo) GitilesCommit() (ret *bbpb.GitilesCommit) {
	panic("implement me")
}

func (b bbInfo) TaskPayload() (cipdPkg, cipdVers string, pathInTask string) {
	panic("implement me")
}
