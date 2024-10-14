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
	"os"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"google.golang.org/protobuf/proto"
	pb "google.golang.org/protobuf/types/descriptorpb"
)

func TestUtil(t *testing.T) {
	t.Parallel()

	ftt.Run("Util", t, func(t *ftt.Test) {
		descFileBytes, err := os.ReadFile("./internal/util.desc")
		assert.Loosely(t, err, should.BeNil)

		var desc pb.FileDescriptorSet
		err = proto.Unmarshal(descFileBytes, &desc)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, desc.File, should.HaveLength(2))
		assert.Loosely(t, desc.File[0].GetName(), should.Equal("google/protobuf/descriptor.proto"))
		file := desc.File[1]
		assert.Loosely(t, file.GetName(), should.Equal("go.chromium.org/luci/common/proto/google/descutil/internal/util.proto"))

		t.Run("Resolve works", func(t *ftt.Test) {
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
				t.Run(n, func(t *ftt.Test) {
					actualFile, obj, _ := Resolve(&desc, n)
					assert.Loosely(t, actualFile, should.Equal(file))
					assert.Loosely(t, obj, should.NotBeNil)
				})
			}

			t.Run("wrong name", func(t *ftt.Test) {
				actualFile, obj, path := Resolve(&desc, "foo")
				assert.Loosely(t, actualFile, should.BeNil)
				assert.Loosely(t, obj, should.BeNil)
				assert.Loosely(t, path, should.BeNil)
			})
		})

		t.Run("IndexSourceCodeInfo", func(t *ftt.Test) {
			sourceCodeInfo, err := IndexSourceCodeInfo(file)
			assert.Loosely(t, err, should.BeNil)
			_, m1, _ := Resolve(&desc, "descutil.M1")
			loc := sourceCodeInfo[m1]
			assert.Loosely(t, loc, should.NotBeNil)
			assert.Loosely(t, loc.GetLeadingComments(), should.Equal(" M1\n next line.\n"))
		})
	})
}
