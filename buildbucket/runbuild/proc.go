// Copyright 2019 The LUCI Authors.
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

package runbuild

import (
	"os/exec"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// userSubprocess is a subprocess of a user executable.
type userSubprocess struct {
	Path      string
	Dir       string
	InitBuild *pb.Build

	cmd *exec.Cmd
}

func (p *userSubprocess) Run(ctx context.Context) error {
	userLUCICtx.SetInCmd(cmd)
	cmd.Dir = userWorkDir
	buildBytes, err := proto.Marshal(args.Build)
	if err != nil {
		return nil, err
	}

}
