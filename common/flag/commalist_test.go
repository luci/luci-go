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

package flag

import (
	"flag"
	"io"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCommaList(t *testing.T) {
	t.Parallel()
	ftt.Run("Given a FlagSet with a CommaList flag", t, func(t *ftt.Test) {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.Usage = func() {}
		fs.SetOutput(io.Discard)
		var s []string
		fs.Var(CommaList(&s), "list", "Some list")
		t.Run("When parsing with flag absent", func(t *ftt.Test) {
			err := fs.Parse([]string{})
			t.Run("The parsed slice should be empty", func(t *ftt.Test) {
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(s), should.BeZero)
			})
		})
		t.Run("When parsing a single item", func(t *ftt.Test) {
			err := fs.Parse([]string{"-list", "foo"})
			t.Run("The parsed slice should contain the item", func(t *ftt.Test) {
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.Match([]string{"foo"}))
			})
		})
		t.Run("When parsing multiple items", func(t *ftt.Test) {
			err := fs.Parse([]string{"-list", "foo,bar,spam"})
			t.Run("The parsed slice should contain the items", func(t *ftt.Test) {
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s, should.Match([]string{"foo", "bar", "spam"}))
			})
		})
	})
}
