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

package rerun

import (
	"context"
	"testing"

	"go.chromium.org/luci/bisection/internal/buildbucket"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
)

func TestRerun(t *testing.T) {
	t.Parallel()

	Convey("getRerunPropertiesAndDimensions", t, func() {
		c := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		c = clock.Set(c, cl)

		// Setup mock for buildbucket
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := buildbucket.NewMockedClient(c, ctl)
		c = mc.Ctx
		bootstrapProperties := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"bs_key_1": structpb.NewStringValue("bs_val_1"),
			},
		}

		targetBuilder := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"builder": structpb.NewStringValue("linux-test"),
				"group":   structpb.NewStringValue("buildergroup1"),
			},
		}

		res := &bbpb.Build{
			Builder: &bbpb.BuilderID{
				Project: "chromium",
				Bucket:  "ci",
				Builder: "linux-test",
			},
			Input: &bbpb.Build_Input{
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"builder_group":         structpb.NewStringValue("buildergroup1"),
						"$bootstrap/properties": structpb.NewStructValue(bootstrapProperties),
						"another_prop":          structpb.NewStringValue("another_val"),
					},
				},
			},
			Infra: &bbpb.BuildInfra{
				Swarming: &bbpb.BuildInfra_Swarming{
					TaskDimensions: []*bbpb.RequestedDimension{
						{
							Key:   "dimen_key_1",
							Value: "dimen_val_1",
						},
						{
							Key:   "os",
							Value: "ubuntu",
						},
						{
							Key:   "gpu",
							Value: "Intel",
						},
					},
				},
			},
		}
		mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()
		extraProps := map[string]interface{}{
			"analysis_id":     4646418413256704,
			"compile_targets": []string{"target"},
			"bisection_host":  "luci-bisection.appspot.com",
		}
		props, dimens, err := getRerunPropertiesAndDimensions(c, 1234, extraProps)
		So(err, ShouldBeNil)
		So(props, ShouldResemble, &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"builder_group":  structpb.NewStringValue("buildergroup1"),
				"target_builder": structpb.NewStructValue(targetBuilder),
				"$bootstrap/properties": structpb.NewStructValue(&structpb.Struct{
					Fields: map[string]*structpb.Value{
						"bs_key_1": structpb.NewStringValue("bs_val_1"),
					},
				}),
				"analysis_id":     structpb.NewNumberValue(4646418413256704),
				"compile_targets": structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("target")}}),
				"bisection_host":  structpb.NewStringValue("luci-bisection.appspot.com"),
			},
		})
		So(dimens, ShouldResemble, []*bbpb.RequestedDimension{
			{
				Key:   "os",
				Value: "ubuntu",
			},
			{
				Key:   "gpu",
				Value: "Intel",
			},
		})
	})
}
