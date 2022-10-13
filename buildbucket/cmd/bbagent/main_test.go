// Copyright 2022 The LUCI Authors.
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

package main

import (
	"context"
	"flag"
	"testing"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/lucictx"
	"go.chromium.org/luci/luciexe"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestReadyToFinalize(t *testing.T) {
	ctx := memory.Use(context.Background())
	ctx = memlogger.Use(ctx)
	outputFlag := luciexe.AddOutputFlagToSet(&flag.FlagSet{})
	finalBuild := &bbpb.Build{}

	Convey("Ready to finalize: success", t, func() {
		isReady := readyToFinalize(ctx, finalBuild, nil, nil, outputFlag)
		So(isReady, ShouldEqual, true)

	})

	Convey("Not ready to finalize: fatal error not nil", t, func() {
		isReady := readyToFinalize(ctx, finalBuild, errors.New("Fatal Error Happened"), nil, outputFlag)
		So(isReady, ShouldEqual, false)

	})
}

func TestBackFillTaskInfo(t *testing.T) {
	Convey("backFillTaskInfo", t, func() {
		ctx := lucictx.SetSwarming(context.Background(), &lucictx.Swarming{
			Task: &lucictx.Task{
				BotDimensions: []string{
					"cpu:x86",
					"cpu:x86-64",
					"id:bot_id",
					"gcp:google.com:chromecompute",
				},
			},
		})

		build := &bbpb.Build{
			Infra: &bbpb.BuildInfra{
				Swarming: &bbpb.BuildInfra_Swarming{},
			},
		}
		input := clientInput{input: &bbpb.BBAgentArgs{Build: build}}

		So(backFillTaskInfo(ctx, input), ShouldEqual, 0)
		So(build.Infra.Swarming.BotDimensions, ShouldResembleProto, []*bbpb.StringPair{
			{
				Key:   "cpu",
				Value: "x86",
			},
			{
				Key:   "cpu",
				Value: "x86-64",
			},
			{
				Key:   "id",
				Value: "bot_id",
			},
			{
				Key:   "gcp",
				Value: "google.com:chromecompute",
			},
		})
	})
}
