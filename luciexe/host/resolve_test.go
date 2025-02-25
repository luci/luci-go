// Copyright 2024 The LUCI Authors.
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

package host

import (
	"os"
	"runtime"
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestResolveExeCmd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unable to resolve luciexe")
	}

	ftt.Run(`test resolve exe cmd`, t, func(t *ftt.Test) {
		opts := &Options{
			BaseBuild: &buildbucketpb.Build{
				Infra: &buildbucketpb.BuildInfra{Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{}},
				Exe: &buildbucketpb.Executable{
					Cmd: []string{"luciexe"},
				},
			},
		}

		t.Run("default", func(t *ftt.Test) {
			args, err := ResolveExeCmd(opts, "/default")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, args, should.Match([]string{"/default/luciexe"}))
		})

		t.Run("agent", func(t *ftt.Test) {
			opts.BaseBuild.Infra.Buildbucket.Agent = &buildbucketpb.BuildInfra_Buildbucket_Agent{
				Purposes: map[string]buildbucketpb.BuildInfra_Buildbucket_Agent_Purpose{
					"inputs": buildbucketpb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
				},
			}
			opts.DownloadAgentInputs = true
			opts.agentInputsDir = "/base"

			args, err := ResolveExeCmd(opts, "/default")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, args, should.Match([]string{"/base/inputs/luciexe"}))
		})

		t.Run("wrapper", func(t *ftt.Test) {
			wrapper, err := os.Executable() // Resolve require wrapper exist; use the test binary.
			assert.Loosely(t, err, should.BeNil)
			opts.BaseBuild.Exe.Wrapper = []string{wrapper}

			args, err := ResolveExeCmd(opts, "/default")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, args, should.Match([]string{wrapper, "--", "/default/luciexe"}))
		})
	})
}
