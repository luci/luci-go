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

// This LUCI executable emits a successful build with an incomplete step.

package main

import (
	pb "go.chromium.org/luci/buildbucket/proto"
)

func main() {
	initExecutable()
	build := client.InitBuild

	// Final build must have a terminal status.
	build.Status = pb.Status_INFRA_FAILURE
	build.Steps = append(build.Steps, &pb.Step{
		Name: "pending step",
	})
	writeBuild(build)
}
