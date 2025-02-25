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

package util

import (
	"regexp"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRegexUtil(t *testing.T) {
	ftt.Run("MatchedNamedGroup", t, func(t *ftt.Test) {
		pattern := regexp.MustCompile(`(?P<g1>test1)(test2)(?P<g2>test3)`)
		matches, err := MatchedNamedGroup(pattern, `test1test2test3`)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, matches, should.Match(map[string]string{
			"g1": "test1",
			"g2": "test3",
		}))
		_, err = MatchedNamedGroup(pattern, `test`)
		assert.Loosely(t, err, should.NotBeNil)
	})
}
