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

package pbutil

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestWorkUnitID(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateWorkUnitID", t, func(t *ftt.Test) {
		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateWorkUnitID("a"), should.BeNil)
			assert.Loosely(t, ValidateWorkUnitID("a:b"), should.BeNil)
			assert.Loosely(t, ValidateWorkUnitID("a-b_c.d"), should.BeNil)
			assert.Loosely(t, ValidateWorkUnitID("a-b_c.d:e-f_g.h"), should.BeNil)
		})

		t.Run("Invalid", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				assert.Loosely(t, ValidateWorkUnitID(""), should.ErrLike("unspecified"))
			})
			t.Run("Uppercase", func(t *ftt.Test) {
				assert.Loosely(t, ValidateWorkUnitID("A"), should.ErrLike("does not match"))
			})
			t.Run("Starts with colon", func(t *ftt.Test) {
				assert.Loosely(t, ValidateWorkUnitID(":b"), should.ErrLike("does not match"))
			})
			t.Run("Ends with colon", func(t *ftt.Test) {
				assert.Loosely(t, ValidateWorkUnitID("a:"), should.ErrLike("does not match"))
			})
			t.Run("Uppercase in second part", func(t *ftt.Test) {
				assert.Loosely(t, ValidateWorkUnitID("a:B"), should.ErrLike("does not match"))
			})
			t.Run("Too long", func(t *ftt.Test) {
				assert.Loosely(t, ValidateWorkUnitID(strings.Repeat("a", workUnitIDMaxLength+1)), should.ErrLike("must be at most 100 bytes long"))
			})
		})
	})
}

func TestWorkUnitName(t *testing.T) {
	t.Parallel()
	ftt.Run("(Try)ParseWorkUnitName", t, func(t *ftt.Test) {
		t.Run("Valid", func(t *ftt.Test) {
			testCase := "rootInvocations/u-rootinv/workUnits/work-unit-1"
			rootInvID, wuID, err := ParseWorkUnitName(testCase)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rootInvID, should.Equal("u-rootinv"))
			assert.Loosely(t, wuID, should.Equal("work-unit-1"))

			rootInvID, wuID, ok := TryParseWorkUnitName(testCase)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, rootInvID, should.Equal("u-rootinv"))
			assert.Loosely(t, wuID, should.Equal("work-unit-1"))
		})

		t.Run("Invalid work unit ID", func(t *ftt.Test) {
			testCase := "rootInvocations/rootinv/workUnits/INVALID_ID"
			_, _, err := ParseWorkUnitName(testCase)
			assert.Loosely(t, err, should.ErrLike(`does not match`))

			_, _, ok := TryParseWorkUnitName(testCase)
			assert.Loosely(t, ok, should.BeFalse)
		})
		t.Run("Invalid root invocation ID", func(t *ftt.Test) {
			testCase := "rootInvocations/INVALID_ID/workUnits/workunit"
			_, _, err := ParseWorkUnitName(testCase)
			assert.Loosely(t, err, should.ErrLike(`does not match`))

			_, _, ok := TryParseWorkUnitName(testCase)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("Too long work unit ID", func(t *ftt.Test) {
			longID := strings.Repeat("a", workUnitIDMaxLength+1)
			testCase := "rootInvocations/rootinv/workUnits/" + longID

			_, _, err := ParseWorkUnitName(testCase)
			assert.Loosely(t, err, should.ErrLike(`work unit ID must be at most 100 bytes long`))

			_, _, ok := TryParseWorkUnitName(testCase)
			assert.Loosely(t, ok, should.BeFalse)
		})
		t.Run("Too long root invocation ID", func(t *ftt.Test) {
			longID := strings.Repeat("a", workUnitIDMaxLength+1)
			testCase := "rootInvocations/" + longID + "/workUnits/workunit"

			_, _, err := ParseWorkUnitName(testCase)
			assert.Loosely(t, err, should.ErrLike(`root invocation ID must be at most 100 bytes long`))

			_, _, ok := TryParseWorkUnitName(testCase)
			assert.Loosely(t, ok, should.BeFalse)
		})
	})

	ftt.Run("WorkUnitName", t, func(t *ftt.Test) {
		assert.Loosely(t, WorkUnitName("rootinv", "work-unit-1"), should.Equal("rootInvocations/rootinv/workUnits/work-unit-1"))
	})

	ftt.Run("ValidateWorkUnitName", t, func(t *ftt.Test) {
		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateWorkUnitName("rootInvocations/a/workUnits/b"), should.BeNil)
		})
		t.Run("Invalid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateWorkUnitName("invocations/a/workUnits/b"), should.ErrLike("does not match"))
		})
	})
}
