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

package swarmingimpl

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestProtoJsonAdapter(t *testing.T) {
	t.Parallel()

	Convey(`Tests that we can create an empty proto from a pointer to the underlying proto type`, t, func() {
		expected := createEmptyUnderlyingProto[*swarmingv2.BotInfo]()
		So(expected, ShouldResembleProto, &swarmingv2.BotInfo{})
	})

	Convey(`Roundtrip to check that parsing ProtoJSONAdapter works as expected`, t, func() {
		expected := `{
 "bot_id": "bot1"
}`
		bot := &swarmingv2.BotInfo{
			BotId: "bot1",
		}
		adapter := ProtoJSONAdapter[*swarmingv2.BotInfo]{
			Proto: bot,
		}
		actual, err := json.MarshalIndent(&adapter, "", DefaultIndent)
		So(err, ShouldBeNil)
		So(string(actual), ShouldEqual, expected)
		adapter = ProtoJSONAdapter[*swarmingv2.BotInfo]{}
		err = json.Unmarshal(actual, &adapter)
		So(err, ShouldBeNil)
		So(bot, ShouldResembleProto, adapter.Proto)
	})

	Convey(`Tests that ProtoJsonAdapter works on lists of protobufs`, t, func() {
		expected := `[
 {
  "bot_id": "bot1"
 }
]`
		bot := &swarmingv2.BotInfo{
			BotId: "bot1",
		}
		adapters := []ProtoJSONAdapter[*swarmingv2.BotInfo]{
			{
				Proto: bot,
			},
		}
		actual, err := json.MarshalIndent(&adapters, "", DefaultIndent)
		So(err, ShouldBeNil)
		So(string(actual), ShouldEqual, expected)
	})
}
