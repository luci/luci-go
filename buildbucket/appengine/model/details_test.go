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

package model

import (
	"context"
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDetails(t *testing.T) {
	t.Parallel()

	Convey("BuildDetails", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		b, err := proto.Marshal(&pb.Build{
			Steps: []*pb.Step{
				{
					Name: "step",
				},
			},
		})
		So(err, ShouldBeNil)

		Convey("ToProto", func() {
			Convey("zipped", func() {
				s := &BuildSteps{
					// { name: "step" }
					Bytes:    []byte{120, 156, 234, 98, 100, 227, 98, 41, 46, 73, 45, 0, 4, 0, 0, 255, 255, 9, 199, 2, 92},
					IsZipped: true,
				}
				p, err := s.ToProto(ctx)
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, []*pb.Step{
					{
						Name: "step",
					},
				})
			})

			Convey("not zipped", func() {
				s := &BuildSteps{
					IsZipped: false,
					Bytes:    b,
				}
				p, err := s.ToProto(ctx)
				So(err, ShouldBeNil)
				So(p, ShouldResembleProto, []*pb.Step{
					{
						Name: "step",
					},
				})
			})
		})
	})
}
