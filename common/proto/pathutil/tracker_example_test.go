// Copyright 2025 The LUCI Authors.
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

package pathutil_test

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/proto/pathutil"
)

var testMessageTrackerFactory = pathutil.TrackerFactory[*TestMessage]{
	AverageRecursiveMessageDepth: 2,
}

type validationContext struct {
	// example external parameters to modify behavior of validation on a per
	// instance basis.
	singleFieldPrefix string
}

func checkMapKey(t *pathutil.Tracker, key string) {
	if key == "" {
		t.Err("empty key not allowed")
	}
}

func checkIntRange(t *pathutil.Tracker, val int64) {
	if val < 0 {
		t.Err("val %d < 0", val)
	}
	if val > 100 {
		t.Err("val %d > 100", val)
	}
}

func validateTestMessage(ctx *validationContext, t *pathutil.Tracker, msg *TestMessage) {
	if msg == nil {
		return
	}
	// messages

	// singular recursion. See doc on pathutil.TrackOptionalMsg for why this is a
	// loop.
	for msg := range pathutil.TrackOptionalMsg(t, "msg", msg.Msg) {
		validateTestMessage(ctx, t, msg)
	}

	const listMsg = "list_msg"
	if l := len(msg.ListMsg); l > 10 {
		t.FieldErr(listMsg, "len(%d) > 10", l)
	}
	for _, val := range pathutil.TrackList(t, listMsg, msg.ListMsg) {
		validateTestMessage(ctx, t, val)
	}

	const mapMsg = "map_msg"
	if l := len(msg.MapMsg); l > 10 {
		t.FieldErr(mapMsg, "len(%d) > 10", l)
	}
	for key, val := range pathutil.TrackMap(t, mapMsg, msg.MapMsg) {
		checkMapKey(t, key)
		validateTestMessage(ctx, t, val)
	}

	// scalars
	if actual := msg.Scalar; !strings.HasPrefix(actual, ctx.singleFieldPrefix) {
		t.FieldErr("scalar", "expected prefix %q, got %q", ctx.singleFieldPrefix, actual)
	}

	const listScalar = "list_scalar"
	if l := len(msg.ListScalar); l > 10 {
		t.FieldErr(listScalar, "len(%d) > 10", l)
	}
	for _, val := range pathutil.TrackList(t, listScalar, msg.ListScalar) {
		checkIntRange(t, val)
	}

	const mapScalar = "map_scalar"
	if l := len(msg.MapScalar); l > 10 {
		t.FieldErr(mapScalar, "len(%d) > 10", l)
	}
	for key, val := range pathutil.TrackMap(t, mapScalar, msg.MapScalar) {
		checkMapKey(t, key)
		checkIntRange(t, val)
	}
}

func ExampleTracker() {
	// Make a new Tracker from our TrackerFactory[*TestMessage]
	t := testMessageTrackerFactory.New(3)

	// Our validation algorithm needs some ambient context
	ctx := &validationContext{singleFieldPrefix: "prefix:"}

	// validate
	validateTestMessage(ctx, t, &TestMessage{
		Scalar:     "bad:prefix",
		ListScalar: []int64{1, 2, 3, 4, 300},
		MapMsg: map[string]*TestMessage{
			"hi": {
				Scalar:     "prefix:OK",
				ListScalar: make([]int64, 20),
				MapMsg: map[string]*TestMessage{
					"deeper": {
						MapMsg: map[string]*TestMessage{
							"danger!": {
								Scalar: "prefix:stuff",
								MapMsg: map[string]*TestMessage{
									"oh no": {},
								},
							},
						},
					},
				},
			},
		},
	})

	// Now look at the accumulated errors.
	fmt.Print(t.Error("msg"))

	// Output:
	// msg.map_msg["hi"].map_msg["deeper"].map_msg["danger!"]: exceeds maximum depth 3
	// msg.map_msg["hi"].map_msg["deeper"].scalar: expected prefix "prefix:", got ""
	// msg.map_msg["hi"].list_scalar: len(20) > 10
	// msg.scalar: expected prefix "prefix:", got "bad:prefix"
	// msg.list_scalar[4]: val 300 > 100
}
