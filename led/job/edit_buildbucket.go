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
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

// RecipeDirectory is a very unfortunate constant which is here for
// a combination of reasons:
//  1. swarming doesn't allow you to 'checkout' an isolate relative to any
//     path in the task (other than the task root). This means that
//     whatever value we pick for EditRecipeBundle must be used EVERYWHERE
//     the isolated hash is used.
//  2. Currently the 'recipe_engine/led' module will blindly take the
//     isolated input and 'inject' it into further uses of led. This module
//     currently doesn't specify the checkout dir, relying on kitchen's
//     default value of (you guessed it) "kitchen-checkout".
//
// In order to fix this (and it will need to be fixed for bbagent support):
//   - The 'recipe_engine/led' module needs to accept 'checkout-dir' as
//     a parameter in its input properties.
//   - led needs to start passing the checkout dir to the led module's input
//     properties.
//   - `led edit` needs a way to manipulate the checkout directory in a job
//   - The 'recipe_engine/led' module needs to set this in the job
//     alongside the isolate hash when it's doing the injection.
//
// For now, we just hard-code it.
//
// TODO(crbug.com/1072117): Fix this, it's weird.
const RecipeDirectory = "kitchen-checkout"

// LEDBuilderIsBootstrappedProperty should be set to a boolean value. If true,
// edit-recipe-bundle will set the "led_cas_recipe_bundle" property
// instead of overwriting the build's payload.
const LEDBuilderIsBootstrappedProperty = "led_builder_is_bootstrapped"

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
			tags := bbe.bb.BbagentArgs.Build.Tags
			for _, tag := range values {
				k, v := strpair.Parse(tag)
				tags = append(tags, &bbpb.StringPair{
					Key:   k,
					Value: v,
				})
			}
			sort.Slice(tags, func(i, j int) bool { return tags[i].Key < tags[j].Key })
			bbe.bb.BbagentArgs.Build.Tags = tags
		}
		return nil
	})
}

func (bbe *buildbucketEditor) TaskPayloadSource(cipdPkg, cipdVers string) {
	bbe.tweak(func() error {
		usedCipdVers := cipdVers
		if cipdVers == "" {
			usedCipdVers = "latest"
		}
		// Update exe.
		exe := bbe.bb.BbagentArgs.Build.Exe
		if cipdPkg != "" {
			exe.CipdPackage = cipdPkg
			exe.CipdVersion = usedCipdVers
		} else if cipdPkg == "" && cipdVers == "" {
			exe.CipdPackage = ""
			exe.CipdVersion = ""
		} else {
			return errors.Reason(
				"cipdPkg and cipdVers must both be set or both be empty: cipdPkg=%q cipdVers=%q",
				cipdPkg, cipdVers).Err()
		}

		// Update infra.Buildbucket.Agent.Input
		if cipdPkg == "" && cipdVers == "" {
			return nil
		}
		bbe.TaskPayloadPath(RecipeDirectory)
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
		if ref, ok := inputData[RecipeDirectory]; ok && ref.GetCipd() != nil {
			if len(ref.GetCipd().Specs) > 1 {
				return errors.Reason("can only have one user payload under %s", RecipeDirectory).Err()
			}
			ref.GetCipd().Specs[0] = &bbpb.InputDataRef_CIPD_PkgSpec{
				Package: cipdPkg,
				Version: usedCipdVers,
			}
			return nil
		}
		inputData[RecipeDirectory] = &bbpb.InputDataRef{
			DataType: &bbpb.InputDataRef_Cipd{
				Cipd: &bbpb.InputDataRef_CIPD{
					Specs: []*bbpb.InputDataRef_CIPD_PkgSpec{
						{
							Package: cipdPkg,
							Version: usedCipdVers,
						},
					},
				},
			},
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
			casInstance := casRef.CasInstance
			if casInstance == "" {
				var err error
				casInstance, err = bbe.jd.CasInstance()
				if err != nil {
					return err
				}
			}
			inputData[path] = &bbpb.InputDataRef{
				DataType: &bbpb.InputDataRef_Cas{
					Cas: &bbpb.InputDataRef_CAS{
						CasInstance: casInstance,
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
		infra := bbe.bb.BbagentArgs.Build.Infra
		if infra.Swarming != nil {
			bbe.bb.BbagentArgs.Build.Infra.Swarming.TaskDimensions = nil
		} else {
			bbe.bb.BbagentArgs.Build.Infra.Backend.TaskDimensions = nil
		}

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

		build := bbe.bb.BbagentArgs.Build
		var curTimeout time.Duration
		if build.SchedulingTimeout != nil {
			if err := build.SchedulingTimeout.CheckValid(); err != nil {
				return err
			}
			curTimeout = build.SchedulingTimeout.AsDuration()
		}
		var maxExp time.Duration
		var newDimLen int
		if build.Infra.Swarming != nil {
			newDimLen = len(build.Infra.Swarming.TaskDimensions) + len(dimEdits)
		} else {
			newDimLen = len(build.Infra.Backend.TaskDimensions) + len(dimEdits)
		}
		newDims := make([]*bbpb.RequestedDimension, 0, newDimLen)
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
				if exp > 0 && exp != curTimeout {
					toAdd.Expiration = durationpb.New(exp)
				}
				newDims = append(newDims, toAdd)
			}
		}
		if build.Infra.Swarming != nil {
			build.Infra.Swarming.TaskDimensions = newDims
		} else {
			build.Infra.Backend.TaskDimensions = newDims
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

		infra := bbe.bb.BbagentArgs.Build.Infra
		if infra.Swarming != nil {
			infra.Swarming.Priority = priority
		} else {
			infra.Backend.Config.Fields["priority"] = structpb.NewNumberValue(float64(priority))
		}
		return nil
	})
}

func (bbe *buildbucketEditor) Properties(props map[string]string, auto bool) {
	if len(props) == 0 {
		return
	}
	bbe.tweak(func() error {
		toWrite := map[string]any{}
		removed := make([]string, 0, len(props))

		for k, v := range props {
			if v == "" {
				toWrite[k] = nil
				removed = append(removed, k)
			} else {
				var obj any
				if err := json.Unmarshal([]byte(v), &obj); err != nil {
					if !auto {
						return err
					}
					obj = v
				}
				toWrite[k] = obj
			}
		}

		if bbe.bb.BbagentArgs.Build.Input.Properties.GetFields()[LEDBuilderIsBootstrappedProperty].GetBoolValue() {
			propCopy := map[string]any{}
			for k, v := range toWrite {
				if v == nil {
					// removed properties are tracked by `removed`.
					continue
				}
				propCopy[k] = v
			}
			toWrite["led_edited_properties"] = propCopy
			toWrite["led_removed_properties"] = removed
		}

		bbe.bb.WriteProperties(toWrite)
		return nil
	})
}

func (bbe *buildbucketEditor) CIPDPkgs(cipdPkgs CIPDPkgs) {
	if len(cipdPkgs) == 0 {
		return
	}

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

		infra := bbe.bb.BbagentArgs.Build.Infra
		if infra.Swarming != nil {
			infra.Swarming.Hostname = host
		} else {
			return errors.New("the build does not run on swarming directly.")
		}
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
		bbe.Experiments(map[string]bool{buildbucket.ExperimentNonProduction: isExperimental})
		return nil
	})
}

func (bbe *buildbucketEditor) Experiments(exps map[string]bool) {
	bbe.tweak(func() error {
		if len(exps) == 0 {
			return nil
		}

		er := bbe.bb.BbagentArgs.Build.Infra.Buildbucket.ExperimentReasons
		if er == nil {
			er = make(map[string]bbpb.BuildInfra_Buildbucket_ExperimentReason)
			bbe.bb.BbagentArgs.Build.Infra.Buildbucket.ExperimentReasons = er
		}
		enabled := stringset.NewFromSlice(bbe.bb.BbagentArgs.Build.Input.Experiments...)
		for k, v := range exps {
			if k == buildbucket.ExperimentNonProduction {
				bbe.bb.BbagentArgs.Build.Input.Experimental = v
			}
			if v {
				enabled.Add(k)
				er[k] = bbpb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED
			} else {
				enabled.Del(k)
				delete(er, k)
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
