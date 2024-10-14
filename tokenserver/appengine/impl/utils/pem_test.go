// Copyright 2016 The LUCI Authors.
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

package utils

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPem(t *testing.T) {
	ftt.Run("DumpPEM/ParsePEM roundtrip", t, func(t *ftt.Test) {
		data := []byte("blah-blah")
		back, err := ParsePEM(DumpPEM(data, "DATA"), "DATA")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, back, should.Resemble(data))
	})

	ftt.Run("ParsePEM wrong header", t, func(t *ftt.Test) {
		data := []byte("blah-blah")
		_, err := ParsePEM(DumpPEM(data, "DATA"), "NOT DATA")
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("ParsePEM not a PEM", t, func(t *ftt.Test) {
		_, err := ParsePEM("blah-blah", "NOT DATA")
		assert.Loosely(t, err, should.NotBeNil)
	})
}
