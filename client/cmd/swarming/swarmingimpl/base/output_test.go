// Copyright 2023 The LUCI Authors.
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

package base

import (
	"testing"

	"google.golang.org/protobuf/proto"

	. "github.com/smartystreets/goconvey/convey"

	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestEncodeJSON(t *testing.T) {
	t.Parallel()

	testMsg1 := &swarmingv2.NewTaskRequest{ParentTaskId: "task1"}
	testMsg2 := &swarmingv2.NewTaskRequest{ParentTaskId: "task2"}

	call := func(a any) string {
		b, err := EncodeJSON(a)
		So(err, ShouldBeNil)
		return string(b)
	}

	Convey("Nil", t, func() {
		So(call(nil), ShouldEqual, "null")
	})

	Convey("Message", t, func() {
		So(call(testMsg1), ShouldEqual, `{
 "parent_task_id": "task1"
}`)
	})

	Convey("LegacyJSON", t, func() {
		So(call(LegacyJSON("hi")), ShouldEqual, `"hi"`)
	})

	Convey("Slice of proto.Message", t, func() {
		So(call([]proto.Message{testMsg1, testMsg2}), ShouldEqual, `[
 {
  "parent_task_id": "task1"
 },
 {
  "parent_task_id": "task2"
 }
]`)
	})

	Convey("Slice of concrete type", t, func() {
		So(call([]*swarmingv2.NewTaskRequest{testMsg1, testMsg2}), ShouldEqual, `[
 {
  "parent_task_id": "task1"
 },
 {
  "parent_task_id": "task2"
 }
]`)
	})

	Convey("Slice of concrete type wrapped in ListWithStdoutProjection", t, func() {
		wrapped := ListWithStdoutProjection([]*swarmingv2.NewTaskRequest{testMsg1, testMsg2}, func(*swarmingv2.NewTaskRequest) string {
			return ""
		})
		So(call(wrapped), ShouldEqual, `[
 {
  "parent_task_id": "task1"
 },
 {
  "parent_task_id": "task2"
 }
]`)
	})

	Convey("Map of proto.Message", t, func() {
		So(call(map[string]proto.Message{"a": testMsg1, "b": testMsg2}), ShouldEqual, `{
 "a": {
  "parent_task_id": "task1"
 },
 "b": {
  "parent_task_id": "task2"
 }
}`)
	})

	Convey("Map of concrete type", t, func() {
		So(call(map[string]*swarmingv2.NewTaskRequest{"a": testMsg1, "b": testMsg2}), ShouldEqual, `{
 "a": {
  "parent_task_id": "task1"
 },
 "b": {
  "parent_task_id": "task2"
 }
}`)
	})

	Convey("Non-proto slice", t, func() {
		So(func() { call([]string{}) }, ShouldPanic)
	})

	Convey("Non-string map key", t, func() {
		So(func() { call(map[int]proto.Message{}) }, ShouldPanic)
	})

	Convey("Non-proto map value", t, func() {
		So(func() { call(map[string]int{}) }, ShouldPanic)
	})
}
