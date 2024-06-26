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

package bqexporter

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGenerateInvocationBQRow(t *testing.T) {
	t.Parallel()

	Convey("prepareInvocationRow", t, func() {
		properties, err := structpb.NewStruct(map[string]interface{}{
			"num_prop":    123,
			"string_prop": "ABC",
		})
		So(err, ShouldBeNil)
		extendedProperties := map[string]*structpb.Struct{
			"a_key": properties,
		}
		inv := &pb.Invocation{
			Name:                "invocations/exported",
			Realm:               "testproject:testrealm",
			CreateTime:          pbutil.MustTimestampProto(testclock.TestRecentTimeUTC),
			Tags:                pbutil.StringPairs("a", "1", "b", "2"),
			FinalizeTime:        pbutil.MustTimestampProto(testclock.TestRecentTimeUTC),
			IncludedInvocations: []string{"invocations/included0", "invocations/included1"},
			IsExportRoot:        true,
			ProducerResource:    "//builds.example.com/builds/1",
			Properties:          properties,
			ExtendedProperties:  extendedProperties,
		}
		row, err := prepareInvocationRow(inv)
		So(err, ShouldBeNil)
		So(row.Project, ShouldEqual, "testproject")
		So(row.Realm, ShouldEqual, "testrealm")
		So(row.Id, ShouldEqual, "exported")
		// Different implementations may use different spacing between
		// json elements. Ignore this.
		rowProp := strings.ReplaceAll(row.Properties, " ", "")
		So(rowProp, ShouldResemble, `{"num_prop":123,"string_prop":"ABC"}`)
		rowExtProp := strings.ReplaceAll(row.ExtendedProperties, " ", "")
		So(rowExtProp, ShouldResemble, `{"a_key":{"num_prop":123,"string_prop":"ABC"}}`)
	})
}
