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
	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/luciexe/exe"
)

// WriteProperties writes an input property on this Buildbucket message.
func (b *Buildbucket) WriteProperties(inputs map[string]interface{}) {
	b.EnsureBasics()

	if err := exe.WriteProperties(b.BbagentArgs.Build.Input.Properties, inputs); err != nil {
		panic(errors.Annotate(err, "impossible").Err())
	}
}

// EnsureBasics ensures that the following fields are non-nil:
//
//	b.BbagentArgs
//	b.BbagentArgs.Build
//	b.BbagentArgs.Build.Exe
//	b.BbagentArgs.Build.Infra
//	b.BbagentArgs.Build.Infra.Buildbucket
//	b.BbagentArgs.Build.Infra.Logdog
//	b.BbagentArgs.Build.Infra.Swarming
//	b.BbagentArgs.Build.Input
//	b.BbagentArgs.Build.Input.Properties
func (b *Buildbucket) EnsureBasics() {
	proto.Merge(b, &Buildbucket{BbagentArgs: &bbpb.BBAgentArgs{Build: &bbpb.Build{
		Exe: &bbpb.Executable{},
		Input: &bbpb.Build_Input{
			Properties: &structpb.Struct{},
		},
		Infra: &bbpb.BuildInfra{
			Buildbucket: &bbpb.BuildInfra_Buildbucket{},
			Swarming:    &bbpb.BuildInfra_Swarming{},
			Logdog:      &bbpb.BuildInfra_LogDog{},
		},
	}}})
}

// UpdateBuildbucketAgent updates or populates b.BbagentArgs.Build.Infra.Buildbucket.Agent.
func (b *Buildbucket) UpdateBuildbucketAgent(updates *bbpb.BuildInfra_Buildbucket_Agent) {
	if b.BbagentArgs.Build.Infra.Buildbucket.GetAgent() == nil {
		b.BbagentArgs.Build.Infra.Buildbucket.Agent = &bbpb.BuildInfra_Buildbucket_Agent{}
	}
	proto.Merge(b.BbagentArgs.Build.Infra.Buildbucket.Agent, updates)
}

func (b *Buildbucket) updateBuildbucketAgentPayloadPath(newPath string) {
	for p, purpose := range b.BbagentArgs.Build.Infra.Buildbucket.GetAgent().GetPurposes() {
		if purpose == bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD {
			delete(b.BbagentArgs.Build.Infra.Buildbucket.Agent.Purposes, p)
		}
	}
	b.UpdateBuildbucketAgent(&bbpb.BuildInfra_Buildbucket_Agent{
		Purposes: map[string]bbpb.BuildInfra_Buildbucket_Agent_Purpose{
			newPath: bbpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
		},
	})
}

// UpdateBuildFromBbagentArgs populates fields in b.BbagentArgs.Build
// from b.BbagentArgs.
func (b *Buildbucket) UpdateBuildFromBbagentArgs() {
	b.EnsureBasics()

	if b.BbagentArgs.Build.Infra.GetBbagent() == nil {
		b.BbagentArgs.Build.Infra.Bbagent = &bbpb.BuildInfra_BBAgent{
			PayloadPath: b.BbagentArgs.PayloadPath,
			CacheDir:    b.BbagentArgs.CacheDir,
		}
	}

	b.BbagentArgs.Build.Infra.Buildbucket.KnownPublicGerritHosts = b.BbagentArgs.KnownPublicGerritHosts
	b.updateBuildbucketAgentPayloadPath(b.BbagentArgs.PayloadPath)
}

// UpdatePayloadPath updates the payload path of the led build.
func (b *Buildbucket) UpdatePayloadPath(newPath string) {
	b.BbagentArgs.PayloadPath = newPath
	if b.BbagentArgs.Build.Infra.GetBbagent() == nil {
		b.BbagentArgs.Build.Infra.Bbagent = &bbpb.BuildInfra_BBAgent{}
	}
	b.BbagentArgs.Build.Infra.Bbagent.PayloadPath = newPath
	b.updateBuildbucketAgentPayloadPath(newPath)
}

// PayloadPath returns the payload path of the led build.
func (b *Buildbucket) PayloadPath() string {
	return protoutil.ExePayloadPath(b.BbagentArgs.GetBuild())
}

func (b *Buildbucket) CacheDir() string {
	return protoutil.CacheDir(b.BbagentArgs.GetBuild())
}
