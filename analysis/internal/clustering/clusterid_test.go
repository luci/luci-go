// Copyright 2022 The LUCI Authors.
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

package clustering

import (
	"encoding/hex"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"testing"
)

func TestValidate(t *testing.T) {
	ftt.Run(`Validate`, t, func(t *ftt.Test) {
		id := ClusterID{
			Algorithm: "blah-v2",
			ID:        hex.EncodeToString([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
		}
		t.Run(`Algorithm missing`, func(t *ftt.Test) {
			id.Algorithm = ""
			err := id.Validate()
			assert.Loosely(t, err, should.ErrLike(`algorithm not valid`))
		})
		t.Run("Algorithm invalid", func(t *ftt.Test) {
			id.Algorithm = "!!!"
			err := id.Validate()
			assert.Loosely(t, err, should.ErrLike(`algorithm not valid`))
		})
		t.Run("ID missing", func(t *ftt.Test) {
			id.ID = ""
			err := id.Validate()
			assert.Loosely(t, err, should.ErrLike(`ID is empty`))
		})
		t.Run("ID invalid", func(t *ftt.Test) {
			id.ID = "!!!"
			err := id.Validate()
			assert.Loosely(t, err, should.ErrLike(`ID is not valid lowercase hexadecimal bytes`))
		})
		t.Run("ID not lowercase", func(t *ftt.Test) {
			id.ID = "AA"
			err := id.Validate()
			assert.Loosely(t, err, should.ErrLike(`ID is not valid lowercase hexadecimal bytes`))
		})
		t.Run("ID too long", func(t *ftt.Test) {
			id.ID = hex.EncodeToString([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17})
			err := id.Validate()
			assert.Loosely(t, err, should.ErrLike(`ID is too long (got 17 bytes, want at most 16 bytes)`))
		})
	})
}
