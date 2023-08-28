// Copyright 2018 The LUCI Authors.
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
	"bytes"
	"context"
	"regexp"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

func botsExpectErr(argv []string, errLike string) {
	b := botsRun{}
	b.Init(&testAuthFlags{})
	fullArgv := append([]string{"-server", "http://localhost:9050"}, argv...)
	err := b.GetFlags().Parse(fullArgv)
	So(err, ShouldBeNil)
	So(b.Parse(), ShouldErrLike, errLike)
}

func TestBotsParse(t *testing.T) {
	Convey(`Make sure that Parse fails with -quiet without -json.`, t, func() {
		botsExpectErr([]string{"-quiet"}, "specify -json")
	})

	Convey(`Make sure that Parse fails with -count and -field.`, t, func() {
		botsExpectErr([]string{"-count", "-field", "myField"}, "-field cannot")
	})
}

func TestFileOutput(t *testing.T) {
	expectedDims := []*swarmingv2.StringPair{
		&swarmingv2.StringPair{
			Key:   "a",
			Value: "b",
		},
		&swarmingv2.StringPair{
			Key:   "c",
			Value: "d",
		},
	}
	expectedCount := &swarmingv2.BotsCount{
		Count: 10,
		Busy:  6,
		Dead:  4,
	}
	expectedBots := []*swarmingv2.BotInfo{
		{
			BotId: "bot1",
		},
		{
			BotId: "bot2",
		},
	}
	ctx := context.Background()
	service := &testService{
		countBots: func(ctx context.Context, dims []*swarmingv2.StringPair) (*swarmingv2.BotsCount, error) {
			for _, dim := range dims {
				So(dim, ShouldBeIn, expectedDims)
			}
			So(dims, ShouldHaveLength, len(expectedDims))
			return expectedCount, nil
		},
		listBots: func(ctx context.Context, dims []*swarmingv2.StringPair) ([]*swarmingv2.BotInfo, error) {
			// Sometimes the argument parser ends up creating these arguments in
			// different order. These two checks ensure that dims is equal to expected dims.
			for _, dim := range dims {
				So(dim, ShouldBeIn, expectedDims)
			}
			So(dims, ShouldHaveLength, len(expectedDims))
			return expectedBots, nil
		},
	}
	Convey(`Count is correctly written out when -count is specified`, t, func() {
		b := botsRun{}
		b.Init(&testAuthFlags{})
		b.dimensions = map[string]string{"a": "b", "c": "d"}
		expected, err := DefaultProtoMarshalOpts.Marshal(expectedCount)
		expected = append(expected, '\n')
		So(err, ShouldBeNil)
		b.count = true
		out := new(bytes.Buffer)
		err = b.bots(ctx, service, out)
		actual := out.Bytes()
		So(err, ShouldBeNil)
		So(actual, ShouldEqual, expected)
	})

	Convey(`List bots is correctly outputted`, t, func() {
		b := botsRun{}
		b.Init(&testAuthFlags{})
		b.dimensions = map[string]string{"a": "b", "c": "d"}
		expected, err := showBots(expectedBots)
		expected = append(expected, '\n')
		So(err, ShouldBeNil)
		out := new(bytes.Buffer)
		b.count = false
		err = b.bots(ctx, service, out)
		So(err, ShouldBeNil)
		actual := out.Bytes()
		So(actual, ShouldEqual, expected)
	})

	Convey(`showBots creates a list of json bots`, t, func() {
		actual, _ := showBots(expectedBots)
		expected := `[
 {
  "bot_id": "bot1"
 },
 {
  "bot_id": "bot2"
 }
]`
		removeSpaces := regexp.MustCompile(`\s+`)
		// MarshalOptions.Marshal (https://pkg.go.dev/google.golang.org/protobuf/encoding/protojson#MarshalOptions)
		// sometimes adds one or two spaces after keys. Since test data is spaceless, rather compare the values
		// after removing all whitespace to just test the unindented json
		So(string(removeSpaces.ReplaceAll(actual, []byte(""))), ShouldEqual, removeSpaces.ReplaceAllString(expected, ""))
	})
}
