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

package main

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSubstitute(t *testing.T) {
	t.Parallel()

	ftt.Run("substitute", t, func(t *ftt.Test) {
		c := context.Background()
		subs := map[string]string{
			"Field1": "Value1",
			"Field2": "Value2",
			"Field3": "Value3",
		}

		t.Run("err", func(t *ftt.Test) {
			s, err := substitute(c, "Test {{", subs)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, s, should.BeEmpty)
		})

		t.Run("partial", func(t *ftt.Test) {
			s, err := substitute(c, "Test {{.Field2}}", subs)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s, should.Equal("Test Value2"))
		})

		t.Run("full", func(t *ftt.Test) {
			s, err := substitute(c, "Test {{.Field1}} {{.Field2}} {{.Field3}}", subs)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s, should.Equal("Test Value1 Value2 Value3"))
		})
	})
}
