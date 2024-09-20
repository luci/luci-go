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

package properties

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSubtract(t *testing.T) {
	type tcase struct {
		a, b   *structpb.Struct
		expect *structpb.Struct
	}

	doit := func(tc tcase) func(t *testing.T) {
		return func(t *testing.T) {
			t.Helper()

			empty := subtractStruct(tc.a, tc.b)
			assert.That(t, empty, should.Equal(len(tc.a.Fields) == 0), truth.LineContext())
			if !empty {
				assert.That(t, tc.a, should.Match(tc.expect), truth.LineContext())
			}
		}
	}

	t.Run(`basic equivalent`, doit(tcase{
		a: mustJSON(`{
			"hello": {"world": 100}
		}`),
		b: mustJSON(`{
			"hello": {"world": 100}
		}`),
	}))

	t.Run(`nested extra`, doit(tcase{
		a: mustJSON(`{
			"hello": {"world": 100, "zap": "wow"}
		}`),
		b: mustJSON(`{
			"hello": {"world": 100}
		}`),
		expect: mustJSON(`{"hello": {"zap": "wow"}}`),
	}))

	t.Run(`list match`, doit(tcase{
		a: mustJSON(`{
			"hello": ["he", {"cool": 10}]
		}`),
		b: mustJSON(`{
			"hello": ["he", {"cool": 10}]
		}`),
	}))

	t.Run(`list extra`, doit(tcase{
		a: mustJSON(`{
			"hello": ["he", {"cool": 10, "what": true}, 100],
			"extra": { "nested": "stuff" }
		}`),
		b: mustJSON(`{
			"hello": ["he", {"cool": 10}, 100],
			"extra": { "nested": "stuff" }
		}`),
		// nulls indicate 'no difference'.
		expect: mustJSON(`{
			"hello": [null, {"what": true}, null]
		}`),
	}))
}
