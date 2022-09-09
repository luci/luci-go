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
	"encoding/json"
	"sort"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/types/known/durationpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

type buildbucketEditor struct {
	jd *Definition
	bb *Buildbucket

	err error
}

var _ HighLevelEditor = (*buildbucketEditor)(nil)

func newBuildbucketEditor(jd *Definition) *buildbucketEditor {
	bb := jd.GetBuildbucket()
	if bb == nil {
		panic(errors.New("impossible: only supported for Buildbucket builds"))
	}
	bb.EnsureBasics()

	return &buildbucketEditor{jd, bb, nil}
}

func (bbe *buildbucketEditor) Close() error {
	return bbe.err
}

func (bbe *buildbucketEditor) tweak(fn func() error) {
	if bbe.err == nil {
		bbe.err = fn()
	}
}

func (bbe *buildbucketEditor) Tags(values []string) {
	if len(values) == 0 {
		return
	}

	bbe.tweak(func() (err error) {
		if err = validateTags(values); err == nil {
			bbe.bb.ExtraTags = append(bbe.bb.ExtraTags, values...)
			sort.Strings(bbe.bb.ExtraTags)
		}
		return nil
	})
}

func (bbe *buildbucketEditor) TaskPayloadSource(cipdPkg, cipdVers string) {
	bbe.tweak(func() error {
		exe := bbe.bb.BbagentArgs.Build.Exe
		if cipdPkg != "" {
			exe.CipdPackage = cipdPkg
			if cipdVers == "" {
				exe.CipdVersion = "latest"
			} else {
				exe.CipdVersion = cipdVers
			}
		} else if cipdPkg == "" && cipdVers == "" {
			exe.CipdPackage = ""
			exe.CipdVersion = ""
		} else {
			return errors.Reason(
				"cipdPkg and cipdVers must both be set or both be empty: cipdPkg=%q cipdVers=%q",
				cipdPkg, cipdVers).Err()
		}
		return nil
	})
}

func (bbe *buildbucketEditor) TaskPayloadPath(path string) {
	bbe.tweak(func() error {
		bbe.bb.UpdatePayloadPath(path)
		return nil
	})
}

func (bbe *buildbucketEditor) CASTaskPayload(path string, casRef *swarmingpb.CASReference) {
	bbe.tweak(func() error {
		if path != "" {
			bbe.TaskPayloadPath(path)
		} else {
			purposes := bbe.bb.BbagentArgs.Build.Infra.Buildbucket.GetAgent().GetPurposes()
			for dir, pur := range purposes {
				if pur == bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD {
					path = dir
					break
				}
			}
		}

		if path == "" {
			return errors.Reason("failed to get exe payload path").Err()
		}

		input := bbe.bb.BbagentArgs.Build.Infra.Buildbucket.Agent.Input
		if input == nil {
			input = &bbpb.BuildInfra_Buildbucket_Agent_Input{}
			bbe.bb.BbagentArgs.Build.Infra.Buildbucket.Agent.Input = input
		}
		inputData := input.GetData()
		if len(inputData) == 0 {
			inputData = make(map[string]*bbpb.InputDataRef)
			input.Data = inputData
		}

		if ref, ok := inputData[path]; ok && ref.GetCas() != nil {
			if casRef.CasInstance != "" {
				ref.GetCas().CasInstance = casRef.CasInstance
			}
			ref.GetCas().Digest = &bbpb.InputDataRef_CAS_Digest{
				Hash:      casRef.GetDigest().GetHash(),
				SizeBytes: casRef.GetDigest().GetSizeBytes(),
			}
		} else {
			inputData[path] = &bbpb.InputDataRef{
				DataType: &bbpb.InputDataRef_Cas{
					Cas: &bbpb.InputDataRef_CAS{
						CasInstance: casRef.CasInstance,
						Digest: &bbpb.InputDataRef_CAS_Digest{
							Hash:      casRef.GetDigest().GetHash(),
							SizeBytes: casRef.GetDigest().GetSizeBytes(),
						},
					},
				},
			}
		}
		return nil
	})
}

func (bbe *buildbucketEditor) TaskPayloadCmd(args []string) {
	bbe.tweak(func() error {
		if len(args) == 0 {
			args = []string{"luciexe"}
		}
		bbe.bb.BbagentArgs.Build.Exe.Cmd = args
		return nil
	})
}

func (bbe *buildbucketEditor) ClearCurrentIsolated() {
	bbe.tweak(func() error {
		agent := bbe.bb.BbagentArgs.Build.GetInfra().GetBuildbucket().GetAgent()
		if agent == nil {
			return nil
		}

		payloadPath := ""
		for p, purpose := range agent.GetPurposes() {
			if purpose == bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD {
				payloadPath = p
				break
			}
		}
		if payloadPath == "" {
			return nil
		}
		inputData := agent.GetInput().GetData()
		if ref, ok := inputData[payloadPath]; ok {
			if ref.GetCas() != nil {
				delete(inputData, payloadPath)
			}
		}
		return nil
	})
}

func (bbe *buildbucketEditor) ClearDimensions() {
	bbe.tweak(func() error {
		bbe.bb.BbagentArgs.Build.Infra.Swarming.TaskDimensions = nil
		return nil
	})
}

func (bbe *buildbucketEditor) SetDimensions(dims ExpiringDimensions) {
	bbe.ClearDimensions()
	dec := DimensionEditCommands{}
	for key, vals := range dims {
		dec[key] = &DimensionEditCommand{SetValues: vals}
	}
	bbe.EditDimensions(dec)
}

func (bbe *buildbucketEditor) EditDimensions(dimEdits DimensionEditCommands) {
	if len(dimEdits) == 0 {
		return
	}

	bbe.tweak(func() error {
		dims, err := bbe.jd.Info().Dimensions()
		if err != nil {
			return err
		}

		dimMap := dims.toLogical()
		dimEdits.apply(dimMap, 0)

		sw := bbe.bb.BbagentArgs.Build.Infra.Swarming
		var maxExp time.Duration
		newDims := make([]*bbpb.RequestedDimension, 0,
			len(sw.TaskDimensions)+len(dimEdits))
		for _, key := range keysOf(dimMap) {
			valueExp := dimMap[key]
			for _, value := range keysOf(valueExp) {
				exp := valueExp[value]
				if exp > maxExp {
					maxExp = exp
				}

				toAdd := &bbpb.RequestedDimension{
					Key:   key,
					Value: value,
				}
				if exp > 0 {
					toAdd.Expiration = durationpb.New(exp)
				}
				newDims = append(newDims, toAdd)
			}
		}
		sw.TaskDimensions = newDims

		build := bbe.bb.BbagentArgs.Build
		var curTimeout time.Duration
		if build.SchedulingTimeout != nil {
			var err error
			if curTimeout, err = ptypes.Duration(build.SchedulingTimeout); err != nil {
				return err
			}
		}
		if maxExp > curTimeout {
			build.SchedulingTimeout = durationpb.New(maxExp)
		}
		return nil
	})
}

func (bbe *buildbucketEditor) Env(env map[string]string) {
	if len(env) == 0 {
		return
	}

	bbe.tweak(func() error {
		updateStringPairList(&bbe.bb.EnvVars, env)
		return nil
	})
}

func (bbe *buildbucketEditor) Priority(priority int32) {
	bbe.tweak(func() error {
		if priority < 0 {
			return errors.Reason("negative Priority argument: %d", priority).Err()
		}

		bbe.bb.BbagentArgs.Build.Infra.Swarming.Priority = priority
		return nil
	})
}

func (bbe *buildbucketEditor) Properties(props map[string]string, auto bool) {
	if len(props) == 0 {
		return
	}
	bbe.tweak(func() error {
		toWrite := map[string]interface{}{}

		for k, v := range props {
			if v == "" {
				toWrite[k] = nil
			} else {
				var obj interface{}
				if err := json.Unmarshal([]byte(v), &obj); err != nil {
					if !auto {
						return err
					}
					obj = v
				}
				toWrite[k] = obj
			}
		}

		bbe.bb.WriteProperties(toWrite)
		return nil
	})
}

func (bbe *buildbucketEditor) CIPDPkgs(cipdPkgs CIPDPkgs) {
	bbe.tweak(func() error {
		if !bbe.bb.BbagentDownloadCIPDPkgs() {
			cipdPkgs.updateCipdPkgs(&bbe.bb.CipdPackages)
			return nil
		}
		return errors.Reason("not supported for Buildbucket v2 builds").Err()
	})
}

func (bbe *buildbucketEditor) SwarmingHostname(host string) {
	bbe.tweak(func() (err error) {
		if host == "" {
			return errors.New("empty SwarmingHostname")
		}

		bbe.bb.BbagentArgs.Build.Infra.Swarming.Hostname = host
		return
	})
}

func (bbe *buildbucketEditor) TaskName(name string) {
	bbe.tweak(func() (err error) {
		bbe.bb.Name = name
		return
	})
}

func (bbe *buildbucketEditor) Experimental(isExperimental bool) {
	bbe.tweak(func() error {
		bbe.bb.BbagentArgs.Build.Input.Experimental = isExperimental
		return nil
	})
}

func (bbe *buildbucketEditor) Experiments(exps map[string]bool) {
	bbe.tweak(func() error {
		enabled := stringset.NewFromSlice(bbe.bb.BbagentArgs.Build.Input.Experiments...)
		for k, v := range exps {
			if v {
				enabled.Add(k)
			} else {
				enabled.Del(k)
			}
		}
		bbe.bb.BbagentArgs.Build.Input.Experiments = enabled.ToSortedSlice()
		return nil
	})
}

func (bbe *buildbucketEditor) PrefixPathEnv(values []string) {
	if len(values) == 0 {
		return
	}

	bbe.tweak(func() error {
		updatePrefixPathEnv(values, &bbe.bb.EnvPrefixes)
		return nil
	})
}

func (bbe *buildbucketEditor) ClearGerritChanges() {
	bbe.tweak(func() error {
		bbe.bb.BbagentArgs.Build.Input.GerritChanges = nil
		return nil
	})
}

func (bbe *buildbucketEditor) AddGerritChange(cl *bbpb.GerritChange) {
	if cl == nil {
		return
	}

	bbe.tweak(func() error {
		gc := &bbe.bb.BbagentArgs.Build.Input.GerritChanges
		for _, change := range *gc {
			if proto.Equal(change, cl) {
				return nil
			}
		}
		*gc = append(*gc, cl)
		return nil
	})
}

func (bbe *buildbucketEditor) RemoveGerritChange(cl *bbpb.GerritChange) {
	if cl == nil {
		return
	}

	bbe.tweak(func() error {
		gc := &bbe.bb.BbagentArgs.Build.Input.GerritChanges
		for idx, change := range *gc {
			if proto.Equal(change, cl) {
				*gc = append((*gc)[:idx], (*gc)[idx+1:]...)
				return nil
			}
		}
		return nil
	})
}

func (bbe *buildbucketEditor) GitilesCommit(commit *bbpb.GitilesCommit) {
	bbe.tweak(func() error {
		bbe.bb.BbagentArgs.Build.Input.GitilesCommit = commit
		return nil
	})
}
