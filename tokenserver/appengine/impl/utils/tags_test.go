// Copyright 2017 The LUCI Authors.
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
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateTags(t *testing.T) {
	ftt.Run("ValidateTags ok", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateTags(nil), should.BeNil)
		assert.Loosely(t, ValidateTags([]string{"k1:v1", "k2:v2"}), should.BeNil)
		assert.Loosely(t, ValidateTags([]string{"k1:v1:more:stuff"}), should.BeNil)
	})

	ftt.Run("ValidateTags errors", t, func(t *ftt.Test) {
		var many []string
		for i := 0; i < maxTagCount+1; i++ {
			many = append(many, "k:v")
		}
		assert.Loosely(t, ValidateTags(many), should.ErrLike("too many tags given"))

		assert.Loosely(t,
			ValidateTags([]string{"k:v", "not-kv"}),
			should.ErrLike(
				"tag #2: not in <key>:<value> form"))
		assert.Loosely(t,
			ValidateTags([]string{strings.Repeat("k", maxTagKeySize+1) + ":v"}),
			should.ErrLike(
				"tag #1: the key length must not exceed 128"))
		assert.Loosely(t,
			ValidateTags([]string{"k:" + strings.Repeat("v", maxTagValueSize+1)}),
			should.ErrLike(
				"tag #1: the value length must not exceed 1024"))
	})
}
