// Copyright 2014 The LUCI Authors.
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

package pkg

import (
	"bytes"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"strings"
	"testing"
)

func TestReadManifest(t *testing.T) {
	t.Parallel()

	var goodManifest = `{
  "format_version": "1",
  "package_name": "package/name"
}`

	ftt.Run("ReadManifest can read valid manifest", t, func(t *ftt.Test) {
		manifest, err := ReadManifest(strings.NewReader(goodManifest))
		assert.Loosely(t, manifest, should.Resemble(Manifest{
			FormatVersion: "1",
			PackageName:   "package/name",
		}))
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("ReadManifest rejects invalid manifest", t, func(t *ftt.Test) {
		manifest, err := ReadManifest(strings.NewReader("I'm not a manifest"))
		assert.Loosely(t, manifest, should.Resemble(Manifest{}))
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("WriteManifest works", t, func(t *ftt.Test) {
		buf := &bytes.Buffer{}
		m := Manifest{
			FormatVersion: "1",
			PackageName:   "package/name",
		}
		assert.Loosely(t, WriteManifest(&m, buf), should.BeNil)
		assert.Loosely(t, string(buf.Bytes()), should.Equal(goodManifest))
	})
}
