// Copyright 2016 The LUCI Authors.
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

package descutil

import (
	"testing"

	"io/ioutil"

	"github.com/golang/protobuf/proto"
	pb "github.com/golang/protobuf/protoc-gen-go/descriptor"
	. "github.com/smartystreets/goconvey/convey"
)

func TestUtil(t *testing.T) {
	t.Parallel()

	Convey("Util", t, func() {
		descFileBytes, err := ioutil.ReadFile("util_test.desc")
		So(err, ShouldBeNil)

		var desc pb.FileDescriptorSet
		err = proto.Unmarshal(descFileBytes, &desc)
		So(err, ShouldBeNil)

		So(desc.File, ShouldHaveLength, 1)
		file := desc.File[0]
		So(file.GetName(), ShouldEqual, "go.chromium.org/luci/common/proto/google/descutil/util_test.proto")

		Convey("Resolve works", func() {
			names := []string{
				"descutil.E1",
				"descutil.E1.V0",

				"descutil.M1",
				"descutil.M1.f1",

				"descutil.M2.f1",
				"descutil.M2.f2",

				"descutil.M3.O1",
				"descutil.M3.f1",
				"descutil.M3.O2",

				"descutil.S1",
				"descutil.S1.R1",
				"descutil.S2.R2",

				"descutil.NestedMessageParent",
				"descutil.NestedMessageParent.NestedMessage",
				"descutil.NestedMessageParent.NestedMessage.f1",
				"descutil.NestedMessageParent.NestedEnum",
				"descutil.NestedMessageParent.NestedEnum.V0",
			}
			for _, n := range names {
				Convey(n, func() {
					actualFile, obj, _ := Resolve(&desc, n)
					So(actualFile, ShouldEqual, file)
					So(obj, ShouldNotBeNil)
				})
			}

			Convey("wrong name", func() {
				actualFile, obj, path := Resolve(&desc, "foo")
				So(actualFile, ShouldBeNil)
				So(obj, ShouldBeNil)
				So(path, ShouldBeNil)
			})
		})
	})
}
