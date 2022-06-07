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

package reflectutil

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestShallowCopy(t *testing.T) {
	t.Parallel()

	Convey(`ShallowCopy`, t, func() {
		Convey(`nil`, func() {
			msg := (*TestShallowCopyMessage)(nil)
			newMsg := ShallowCopy(msg).(*TestShallowCopyMessage)
			So(newMsg, ShouldBeNil)
		})

		Convey(`basic fields`, func() {
			msg := &TestShallowCopyMessage{
				Field: "hello",
				RepeatedField: []string{
					"this", "is", "a", "test",
				},
				MappedField: map[string]string{
					"this": "is",
					"a":    "test",
				},
			}
			newMsg := ShallowCopy(msg).(*TestShallowCopyMessage)
			So(newMsg, ShouldResembleProto, msg)

			msg.RepeatedField[0] = "meepmorp"
			So(newMsg.RepeatedField[0], ShouldEqual, "meepmorp")

			msg.MappedField["this"] = "meepmorp"
			So(newMsg.MappedField["this"], ShouldEqual, "meepmorp")
		})

		Convey(`msg fields`, func() {
			msg := &TestShallowCopyMessage{
				InnerMsg: &TestShallowCopyMessage_Inner{Field: "hello"},
				RepeatedMsg: []*TestShallowCopyMessage_Inner{
					{Field: "this"}, {Field: "is"}, {Field: "a"}, {Field: "test"},
				},
				MappedMsg: map[string]*TestShallowCopyMessage_Inner{
					"this": {Field: "is"},
					"a":    {Field: "test"},
				},
			}
			newMsg := ShallowCopy(msg).(*TestShallowCopyMessage)
			So(newMsg, ShouldResembleProto, msg)

			msg.InnerMsg.Field = "meepmorp"
			So(newMsg.InnerMsg.Field, ShouldEqual, "meepmorp")

			msg.RepeatedMsg[0].Field = "meepmorp"
			So(newMsg.RepeatedMsg[0].Field, ShouldEqual, "meepmorp")

			msg.MappedMsg["this"].Field = "meepmorp"
			So(newMsg.MappedMsg["this"].Field, ShouldEqual, "meepmorp")
		})
	})
}
