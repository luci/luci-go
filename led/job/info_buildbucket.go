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
	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

type bbInfo struct {
	*Buildbucket

	casUserPayload *swarmingpb.CASReference
}

var _ Info = bbInfo{}

func (b bbInfo) SwarmingHostname() string {
	return b.GetBbagentArgs().GetBuild().GetInfra().GetSwarming().GetHostname()
}

func (b bbInfo) TaskName() string {
	return b.GetName()
}

func (b bbInfo) CurrentIsolated() (*isolated, error) {
	isolated := &isolated{}
	if b.casUserPayload.GetDigest().GetHash() != "" {
		isolated.CASReference = proto.Clone(b.casUserPayload).(*swarmingpb.CASReference)
	}
	return isolated, nil
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

func (b bbInfo) Experiments() (ret []string) {
	if exps := b.GetBbagentArgs().GetBuild().GetInput().GetExperiments(); len(exps) > 0 {
		ret = make([]string, len(exps))
		copy(ret, exps)
	}
	return
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
			ret[i] = proto.Clone(change).(*bbpb.GerritChange)
		}
	}
	return
}

func (b bbInfo) GitilesCommit() (ret *bbpb.GitilesCommit) {
	if gc := b.GetBbagentArgs().GetBuild().GetInput().GetGitilesCommit(); gc != nil {
		ret = proto.Clone(gc).(*bbpb.GitilesCommit)
	}
	return
}

func (b bbInfo) TaskPayloadSource() (cipdPkg, cipdVers string) {
	exe := b.GetBbagentArgs().GetBuild().GetExe()
	cipdPkg = exe.GetCipdPackage()
	cipdVers = exe.GetCipdVersion()
	return
}

func (b bbInfo) TaskPayloadPath() (path string) {
	return b.GetBbagentArgs().PayloadPath
}

func (b bbInfo) TaskPayloadCmd() (args []string) {
	args = append(args, b.GetBbagentArgs().GetBuild().GetExe().GetCmd()...)
	return
}
