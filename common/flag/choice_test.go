// Copyright 2019 The LUCI Authors.
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

package flag

import (
	"flag"
	"io"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestChoice(t *testing.T) {
	t.Parallel()
	ftt.Run("Given a FlagSet with a Choice flag", t, func(t *ftt.Test) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.Usage = func() {}
		fs.SetOutput(io.Discard)
		var s string
		fs.Var(NewChoice(&s, "Apple", "Orange"), "fruit", "")
		t.Run("When parsing with flag absent", func(t *ftt.Test) {
			err := fs.Parse([]string{})
			t.Run("The parsed string should be empty", func(t *ftt.Test) {
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.BeEmpty)
			})
		})
		t.Run("When parsing a valid choice", func(t *ftt.Test) {
			err := fs.Parse([]string{"-fruit", "Orange"})
			t.Run("The parsed flag equals that choice", func(t *ftt.Test) {
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.Equal("Orange"))
			})
		})
		t.Run("When parsing an invalid choice", func(t *ftt.Test) {
			err := fs.Parse([]string{"-fruit", "Onion"})
			t.Run("An error is returned", func(t *ftt.Test) {
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, s, should.BeEmpty)
			})
		})
	})
}
