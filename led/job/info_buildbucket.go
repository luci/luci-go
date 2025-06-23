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

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

type bbInfo struct {
	*Buildbucket
}

var _ Info = bbInfo{}

func (b bbInfo) SwarmingHostname() string {
	if b.GetBbagentArgs().GetBuild().GetInfra().GetSwarming() != nil {
		return b.GetBbagentArgs().GetBuild().GetInfra().GetSwarming().GetHostname()
	}
	backendTarget := b.GetBbagentArgs().GetBuild().GetInfra().GetBackend().GetTask().GetId().GetTarget()
	if backendTarget == "" {
		return ""
	}
	if strings.HasPrefix(backendTarget, "swarming://") {
		return fmt.Sprintf("%s.appspot.com", strings.TrimPrefix(backendTarget, "swarming://"))
	}
	return ""
}

func (b bbInfo) TaskName() string {
	return b.GetName()
}

func (b bbInfo) CurrentIsolated() (*swarmingpb.CASReference, error) {
	cas, _ := b.Payload()
	if cas != nil {
		return &swarmingpb.CASReference{
			CasInstance: cas.GetCasInstance(),
			Digest: &swarmingpb.Digest{
				Hash:      cas.GetDigest().GetHash(),
				SizeBytes: cas.GetDigest().GetSizeBytes(),
			},
		}, nil
	}
	return nil, nil
}

func (b bbInfo) Dimensions() (ExpiringDimensions, error) {
	ldims := logicalDimensions{}
	var dimensions []*bbpb.RequestedDimension
	if b.BbagentArgs.Build.Infra.Swarming != nil {
		dimensions = b.BbagentArgs.Build.Infra.Swarming.TaskDimensions
	} else {
		dimensions = b.BbagentArgs.Build.Infra.Backend.GetTaskDimensions()
	}
	for _, reqDim := range dimensions {
		exp := reqDim.Expiration
		if exp == nil {
			exp = b.BbagentArgs.Build.SchedulingTimeout
		}
		ldims.update(reqDim.Key, reqDim.Value, exp)
	}
	return ldims.toExpiringDimensions(), nil
}

func (b bbInfo) CIPDPkgs() (ret CIPDPkgs, err error) {
	if !b.BbagentDownloadCIPDPkgs() {
		ret = CIPDPkgs{}
		ret.fromList(b.CipdPackages)
		return
	}
	return nil, errors.New("not supported for Buildbucket v2 builds")
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
			ret = make([]string, len(keyVals.Value))
			copy(ret, keyVals.Value)
			break
		}
	}
	return
}

func (b bbInfo) Tags() (ret []string) {
	lenTags := len(b.GetBbagentArgs().GetBuild().GetTags())
	if lenTags > 0 {
		ret = make([]string, len(b.ExtraTags))
		for _, t := range b.BbagentArgs.Build.Tags {
			ret = append(ret, strpair.Format(t.Key, t.Value))
		}
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
	_, cipd := b.Payload()
	if len(cipd.GetSpecs()) > 0 {
		cipdPkg = cipd.Specs[0].GetPackage()
		cipdVers = cipd.Specs[0].GetVersion()
		return
	}
	// Fall back to getting payload from exe.
	exe := b.GetBbagentArgs().GetBuild().GetExe()
	cipdPkg = exe.GetCipdPackage()
	cipdVers = exe.GetCipdVersion()
	return
}

func (b bbInfo) TaskPayloadPath() (path string) {
	return b.PayloadPath()
}

func (b bbInfo) TaskPayloadCmd() (args []string) {
	args = append(args, b.GetBbagentArgs().GetBuild().GetExe().GetCmd()...)
	return
}
