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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSink(t *testing.T) {
	t.Parallel()

	withSink := func(cb func(sink *Sink)) string {
		buf := &bytes.Buffer{}
		sink := NewSink(buf)
		cb(sink)
		assert.Loosely(t, sink.Finalize(), should.BeNil)
		fmt.Printf("%s", buf.String())
		return buf.String()
	}

	ftt.Run("Proto", t, func(t *ftt.Test) {
		out := withSink(func(sink *Sink) {
			assert.Loosely(t, Proto(sink, &clipb.ResultSummaryEntry{
				Output:  "hi",
				Outputs: []string{"a", "b", "c"},
			}), should.BeNil)
		})
		assert.Loosely(t, out, should.Equal(`{
 "output": "hi",
 "outputs": [
  "a",
  "b",
  "c"
 ]
}
`))
	})

	ftt.Run("JSON", t, func(t *ftt.Test) {
		out := withSink(func(sink *Sink) {
			assert.Loosely(t, JSON(sink, map[string]int{"a": 1, "b": 2}), should.BeNil)
		})
		assert.Loosely(t, out, should.Equal(`{
 "a": 1,
 "b": 2
}
`))
	})

	ftt.Run("List", t, func(t *ftt.Test) {
		t.Run("Zero", func(t *ftt.Test) {
			out := withSink(func(sink *Sink) {
				assert.Loosely(t, List[*clipb.ResultSummaryEntry](sink, nil), should.BeNil)
			})
			assert.Loosely(t, out, should.Equal("[]\n"))
		})

		t.Run("One", func(t *ftt.Test) {
			out := withSink(func(sink *Sink) {
				assert.Loosely(t, List(sink, []*clipb.ResultSummaryEntry{
					{
						Output:  "hi 1",
						Outputs: []string{"a", "b", "c"},
					},
				}), should.BeNil)
			})
			assert.Loosely(t, out, should.Equal(`[
 {
  "output": "hi 1",
  "outputs": [
   "a",
   "b",
   "c"
  ]
 }
]
`))
		})

		t.Run("Many", func(t *ftt.Test) {
			out := withSink(func(sink *Sink) {
				assert.Loosely(t, List(sink, []*clipb.ResultSummaryEntry{
					{
						Output:  "hi 1",
						Outputs: []string{"a", "b", "c"},
					},
					{
						Output:  "hi 2",
						Outputs: []string{"d", "e", "f"},
					},
				}), should.BeNil)
			})
			assert.Loosely(t, out, should.Equal(`[
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
`))
		})
	})

	ftt.Run("Map", t, func(t *ftt.Test) {
		t.Run("Zero", func(t *ftt.Test) {
			out := withSink(func(sink *Sink) {
				assert.Loosely(t, Map[*clipb.ResultSummaryEntry](sink, nil), should.BeNil)
			})
			assert.Loosely(t, out, should.Equal("{}\n"))
		})

		t.Run("One", func(t *ftt.Test) {
			out := withSink(func(sink *Sink) {
				assert.Loosely(t, Map(sink, map[string]*clipb.ResultSummaryEntry{
					"a": {
						Output:  "hi 1",
						Outputs: []string{"a", "b", "c"},
					},
				}), should.BeNil)
			})
			assert.Loosely(t, out, should.Equal(`{
 "a": {
  "output": "hi 1",
  "outputs": [
   "a",
   "b",
   "c"
  ]
 }
}
`))
		})

		t.Run("Many", func(t *ftt.Test) {
			out := withSink(func(sink *Sink) {
				assert.Loosely(t, Map(sink, map[string]*clipb.ResultSummaryEntry{
					"a": {
						Output:  "hi 1",
						Outputs: []string{"a", "b", "c"},
					},
					"b": {
						Output:  "hi 2",
						Outputs: []string{"d", "e", "f"},
					},
				}), should.BeNil)
			})
			assert.Loosely(t, out, should.Equal(`{
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
`))
		})
	})
}
