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

package model

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestQuarantineMessage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		val any
		msg string
	}{
		{"yes", "yes"},
		{"", ""},
		{true, GenericQuarantineMessage},
		{false, ""},
		{float64(1.0), GenericQuarantineMessage},
		{float64(0.0), ""},
		{[]any{123}, GenericQuarantineMessage},
		{[]any{}, ""},
		{nil, ""},
	}
	for _, cs := range cases {
		assert.That(t, QuarantineMessage(cs.val), should.Equal(cs.msg), truth.Explain("%q", cs.val))
	}
}
