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

package output

import (
	"bytes"
	"fmt"
	"testing"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/clipb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSink(t *testing.T) {
	t.Parallel()

	withSink := func(cb func(sink *Sink)) string {
		buf := &bytes.Buffer{}
		sink := NewSink(buf)
		cb(sink)
		So(sink.Finalize(), ShouldBeNil)
		fmt.Printf("%s", buf.String())
		return buf.String()
	}

	Convey("Proto", t, func() {
		out := withSink(func(sink *Sink) {
			So(Proto(sink, &clipb.ResultSummaryEntry{
				Output:  "hi",
				Outputs: []string{"a", "b", "c"},
			}), ShouldBeNil)
		})
		So(out, ShouldEqual, `{
 "output": "hi",
 "outputs": [
  "a",
  "b",
  "c"
 ]
}
`)
	})

	Convey("JSON", t, func() {
		out := withSink(func(sink *Sink) {
			So(JSON(sink, map[string]int{"a": 1, "b": 2}), ShouldBeNil)
		})
		So(out, ShouldEqual, `{
 "a": 1,
 "b": 2
}
`)
	})

	Convey("List", t, func() {
		Convey("Zero", func() {
			out := withSink(func(sink *Sink) {
				So(List[*clipb.ResultSummaryEntry](sink, nil), ShouldBeNil)
			})
			So(out, ShouldEqual, "[]\n")
		})

		Convey("One", func() {
			out := withSink(func(sink *Sink) {
				So(List(sink, []*clipb.ResultSummaryEntry{
					{
						Output:  "hi 1",
						Outputs: []string{"a", "b", "c"},
					},
				}), ShouldBeNil)
			})
			So(out, ShouldEqual, `[
 {
  "output": "hi 1",
  "outputs": [
   "a",
   "b",
   "c"
  ]
 }
]
`)
		})

		Convey("Many", func() {
			out := withSink(func(sink *Sink) {
				So(List(sink, []*clipb.ResultSummaryEntry{
					{
						Output:  "hi 1",
						Outputs: []string{"a", "b", "c"},
					},
					{
						Output:  "hi 2",
						Outputs: []string{"d", "e", "f"},
					},
				}), ShouldBeNil)
			})
			So(out, ShouldEqual, `[
 {
  "output": "hi 1",
  "outputs": [
   "a",
   "b",
   "c"
  ]
 },
 {
  "output": "hi 2",
  "outputs": [
   "d",
   "e",
   "f"
  ]
 }
]
`)
		})
	})

	Convey("Map", t, func() {
		Convey("Zero", func() {
			out := withSink(func(sink *Sink) {
				So(Map[*clipb.ResultSummaryEntry](sink, nil), ShouldBeNil)
			})
			So(out, ShouldEqual, "{}\n")
		})

		Convey("One", func() {
			out := withSink(func(sink *Sink) {
				So(Map(sink, map[string]*clipb.ResultSummaryEntry{
					"a": {
						Output:  "hi 1",
						Outputs: []string{"a", "b", "c"},
					},
				}), ShouldBeNil)
			})
			So(out, ShouldEqual, `{
 "a": {
  "output": "hi 1",
  "outputs": [
   "a",
   "b",
   "c"
  ]
 }
}
`)
		})

		Convey("Many", func() {
			out := withSink(func(sink *Sink) {
				So(Map(sink, map[string]*clipb.ResultSummaryEntry{
					"a": {
						Output:  "hi 1",
						Outputs: []string{"a", "b", "c"},
					},
					"b": {
						Output:  "hi 2",
						Outputs: []string{"d", "e", "f"},
					},
				}), ShouldBeNil)
			})
			So(out, ShouldEqual, `{
 "a": {
  "output": "hi 1",
  "outputs": [
   "a",
   "b",
   "c"
  ]
 },
 "b": {
  "output": "hi 2",
  "outputs": [
   "d",
   "e",
   "f"
  ]
 }
}
`)
		})
	})
}
