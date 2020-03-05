// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"
	bbpb "go.chromium.org/luci/buildbucket/proto"
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
//   b.BbagentArgs
//   b.BbagentArgs.Build
//   b.BbagentArgs.Build.Input
//   b.BbagentArgs.Build.Input.Properties
//   b.BbagentArgs.Build.Infra
//   b.BbagentArgs.Build.Infra.Swarming
//   b.BbagentArgs.Build.Infra.Logdog
func (b *Buildbucket) EnsureBasics() {
	proto.Merge(b, &Buildbucket{BbagentArgs: &bbpb.BBAgentArgs{Build: &bbpb.Build{
		Input: &bbpb.Build_Input{
			Properties: &structpb.Struct{},
		},
		Infra: &bbpb.BuildInfra{
			Swarming: &bbpb.BuildInfra_Swarming{},
			Logdog:   &bbpb.BuildInfra_LogDog{},
		},
	}}})
}
