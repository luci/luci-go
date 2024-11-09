// Copyright 2021 The LUCI Authors.
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

package metadata

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/changelist"
)

func TestExtract(t *testing.T) {
	t.Parallel()

	ftt.Run("Extract works", t, func(t *ftt.Test) {

		t.Run("Empty", func(t *ftt.Test) {
			assert.Loosely(t, Extract(`Title.`), should.Resemble([]*changelist.StringPair{}))
		})

		t.Run("Git-style", func(t *ftt.Test) {
			assert.Loosely(t, Extract(strings.TrimSpace(`
Title.

Footer: value
No-rma-lIzes: but Only key.
`)), should.Resemble([]*changelist.StringPair{
				{Key: "Footer", Value: "value"},
				{Key: "No-Rma-Lizes", Value: "but Only key."},
			}))
		})

		t.Run("TAGS=sTyLe", func(t *ftt.Test) {
			assert.Loosely(t, Extract(strings.TrimSpace(`
TAG=can BE anywhere
`)), should.Resemble([]*changelist.StringPair{
				{Key: "TAG", Value: "can BE anywhere"},
			}))
		})

		t.Run("Ignores incorrect tags and footers", func(t *ftt.Test) {
			assert.Loosely(t, Extract(strings.TrimSpace(`
Tag=must have UPPEPCASE_KEY.

Footers: must reside in the last paragraph, not above it.

Footer-key must-have-not-spaces: but this one does.
`)), should.Resemble([]*changelist.StringPair{}))
		})

		t.Run("Sorts by keys only, keeps values ordered from last to first", func(t *ftt.Test) {
			assert.Loosely(t, Extract(strings.TrimSpace(`
TAG=first
TAG=second

Footer: A
TAG=third
Footer: D
TAG=fourth
Footer: B
`)), should.Resemble([]*changelist.StringPair{
				{Key: "Footer", Value: "B"},
				{Key: "Footer", Value: "D"},
				{Key: "Footer", Value: "A"},
				{Key: "TAG", Value: "fourth"},
				{Key: "TAG", Value: "third"},
				{Key: "TAG", Value: "second"},
				{Key: "TAG", Value: "first"},
			}))
		})
	})
}
