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
	"testing"

	structpb "github.com/golang/protobuf/ptypes/struct"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBBEnsureBasics(t *testing.T) {
	t.Parallel()

	Convey(`Buildbucket.EnsureBasics`, t, func() {
		jd := testBBJob()
		So(jd.GetBuildbucket().GetBbagentArgs().GetBuild(), ShouldBeNil)

		jd.GetBuildbucket().EnsureBasics()

		So(jd.GetBuildbucket().BbagentArgs.Build.Infra, ShouldNotBeNil)
	})
}

func TestWriteProperties(t *testing.T) {
	t.Parallel()

	Convey(`Buildbucket.WriteProperties`, t, func() {
		jd := testBBJob()
		So(jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetInput().GetProperties(), ShouldBeNil)

		jd.GetBuildbucket().WriteProperties(map[string]interface{}{
			"hello": "world",
		})
		So(jd.GetBuildbucket().GetBbagentArgs().GetBuild().GetInput().GetProperties(), ShouldResemble, &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"hello": {Kind: &structpb.Value_StringValue{StringValue: "world"}},
			},
		})
	})
}
