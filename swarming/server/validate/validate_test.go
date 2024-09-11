// Copyright 2024 The LUCI Authors.
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

package validate

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDimensionKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dim string
		err any
	}{
		{"good", nil},
		{strings.Repeat("a", maxDimensionKeyLen), nil},
		{"", "cannot be empty"},
		{strings.Repeat("a", maxDimensionKeyLen+1), "should be no longer"},
		{"bad key", "should match"},
	}

	for _, cs := range cases {
		t.Run(cs.dim, func(t *testing.T) {
			assert.That(t, DimensionKey(cs.dim), should.ErrLike(cs.err))
		})
	}
}

func TestDimensionValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dim string
		err any
	}{
		{"good value", nil},
		{strings.Repeat("a", maxDimensionValLen), nil},
		{"", "cannot be empty"},
		{strings.Repeat("a", maxDimensionValLen+1), "should be no longer"},
		{" bad value", "no leading or trailing spaces"},
		{"bad value ", "no leading or trailing spaces"},
	}

	for _, cs := range cases {
		t.Run(cs.dim, func(t *testing.T) {
			assert.That(t, DimensionValue(cs.dim), should.ErrLike(cs.err))
		})
	}
}
