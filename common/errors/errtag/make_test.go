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

package errtag_test

import (
	"errors"
	"testing"

	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMake(t *testing.T) {
	t.Parallel()

	t.Run(`ok`, func(t *testing.T) {
		mtag := errtag.Make(`TestMake.ok`, 10)
		assert.That(t, mtag.ValueOrDefault(errors.New("hey")), should.Equal(10))
	})

	t.Run(`default construction panics`, func(t *testing.T) {
		badTag := errtag.Tag[string]{}

		fns := []func(){
			func() { badTag.Value(nil) },
			func() { badTag.Key() },
			func() { badTag.ValueOrDefault(nil) },
			func() { badTag.In(nil) },
			func() { badTag.WithDefault("yo") },
			func() { badTag.Apply(nil) },
			func() { badTag.ApplyValue(nil, "yo") },
			func() { badTag.GenerateErrorTagValue() },
		}

		for i, fn := range fns {
			assert.That(t, fn, should.PanicLikeString("default-constructed"),
				truth.Explain("idx: %d", i))
		}
	})
}
