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

	structpb "github.com/golang/protobuf/ptypes/struct"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
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

func TestExtendedPropertiesEqual(t *testing.T) {
	t.Parallel()
	ftt.Run("ExtendedPropertiesEqual", t, func(t *ftt.Test) {
		struct1 := &structpb.Struct{Fields: map[string]*structpb.Value{"key": {Kind: &structpb.Value_StringValue{StringValue: "val1"}}}}
		struct2 := &structpb.Struct{Fields: map[string]*structpb.Value{"key": {Kind: &structpb.Value_StringValue{StringValue: "val2"}}}}
		struct1Clone := &structpb.Struct{Fields: map[string]*structpb.Value{"key": {Kind: &structpb.Value_StringValue{StringValue: "val1"}}}}

		mapA := map[string]*structpb.Struct{"a": struct1}
		mapAClone := map[string]*structpb.Struct{"a": struct1Clone}
		mapB := map[string]*structpb.Struct{"b": struct1}
		mapC := map[string]*structpb.Struct{"a": struct2}
		mapD := map[string]*structpb.Struct{"a": struct1, "b": struct2}

		t.Run("equal", func(t *ftt.Test) {
			assert.Loosely(t, ExtendedPropertiesEqual(mapA, mapAClone), should.BeTrue)
		})
		t.Run("both nil", func(t *ftt.Test) {
			assert.Loosely(t, ExtendedPropertiesEqual(nil, nil), should.BeTrue)
		})
		t.Run("one nil", func(t *ftt.Test) {
			assert.Loosely(t, ExtendedPropertiesEqual(mapA, nil), should.BeFalse)
			assert.Loosely(t, ExtendedPropertiesEqual(nil, mapA), should.BeFalse)
		})
		t.Run("different length", func(t *ftt.Test) {
			assert.Loosely(t, ExtendedPropertiesEqual(mapA, mapD), should.BeFalse)
		})
		t.Run("different keys", func(t *ftt.Test) {
			assert.Loosely(t, ExtendedPropertiesEqual(mapA, mapB), should.BeFalse)
		})
		t.Run("different values", func(t *ftt.Test) {
			assert.Loosely(t, ExtendedPropertiesEqual(mapA, mapC), should.BeFalse)
		})
	})
}

func TestValidateWorkUnitState(t *testing.T) {
	ftt.Run(`ValidateWorkUnitState`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateWorkUnitState(pb.WorkUnit_RUNNING), should.BeNil)
			assert.Loosely(t, ValidateWorkUnitState(pb.WorkUnit_SUCCEEDED), should.BeNil)
		})
		t.Run(`Unspecified`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateWorkUnitState(pb.WorkUnit_STATE_UNSPECIFIED), should.ErrLike("unspecified"))
		})
		t.Run(`Invalid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateWorkUnitState(pb.WorkUnit_FINAL_STATE_MASK), should.ErrLike("FINAL_STATE_MASK is not a valid state"))
			assert.Loosely(t, ValidateWorkUnitState(pb.WorkUnit_State(999)), should.ErrLike("unknown state 999"))
		})
	})
}

func TestValidateSummaryMarkdown(t *testing.T) {
	ftt.Run(`ValidateSummaryMarkdown`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateSummaryMarkdown("hello world", true), should.BeNil)
			assert.Loosely(t, ValidateSummaryMarkdown(strings.Repeat("a", summaryMarkdownMaxLength), true), should.BeNil)
			assert.Loosely(t, ValidateSummaryMarkdown(strings.Repeat("a", summaryMarkdownMaxLength+1), false), should.BeNil)
		})
		t.Run(`Too long`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateSummaryMarkdown(strings.Repeat("a", summaryMarkdownMaxLength+1), true), should.ErrLike("must be at most 4096 bytes long"))
			assert.Loosely(t, ValidateSummaryMarkdown(strings.Repeat("a", summaryMarkdownMaxLength+1), false), should.BeNil)
		})
		t.Run(`Invalid UTF-8`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateSummaryMarkdown("\xff", true), should.ErrLike("not valid UTF-8"))
		})
	})
}

func TestTruncateSummaryMarkdown(t *testing.T) {
	t.Parallel()
	ftt.Run(`TruncateSummaryMarkdown`, t, func(t *ftt.Test) {
		t.Run(`No truncation needed`, func(t *ftt.Test) {
			inputs := []string{
				"hello world",
				strings.Repeat("a", summaryMarkdownMaxLength),
				// € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
				// 1365 * 3 + 1 = 4096.
				strings.Repeat("€", 1365) + "a",
			}
			for _, input := range inputs {
				assert.Loosely(t, TruncateSummaryMarkdown(input), should.Equal(input))
			}
		})
		t.Run(`Truncation needed`, func(t *ftt.Test) {
			assert.Loosely(t, TruncateSummaryMarkdown(strings.Repeat("a", summaryMarkdownMaxLength+1)),
				should.Equal(strings.Repeat("a", summaryMarkdownMaxLength-3)+"..."))

			// Test with multi-byte characters
			// € is 0xE2 0x82 0xAC in UTF-8 (3 bytes).
			// 1365 * 3 = 4095.
			// With two "aa"s it becomes 4097, thus requiring truncation.
			assert.Loosely(t, TruncateSummaryMarkdown(strings.Repeat("€", 1365)+"aa"),
				should.Equal(strings.Repeat("€", 1364)+"..."))
		})
	})
}

func TestValidateModuleShardKey(t *testing.T) {
	t.Parallel()
	ftt.Run(`ValidateModuleShardKey`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateModuleShardKey("a"), should.BeNil)
			assert.Loosely(t, ValidateModuleShardKey("a-z_0-9"), should.BeNil)
			assert.Loosely(t, ValidateModuleShardKey("01234567890abcdef"), should.BeNil)
			assert.Loosely(t, ValidateModuleShardKey(strings.Repeat("a", moduleShardKeyMaxLength)), should.BeNil)
		})
		t.Run(`Invalid`, func(t *ftt.Test) {
			t.Run(`Empty`, func(t *ftt.Test) {
				assert.Loosely(t, ValidateModuleShardKey(""), should.ErrLike("unspecified"))
			})
			t.Run(`Wrong alphabet`, func(t *ftt.Test) {
				// Uppercase
				assert.Loosely(t, ValidateModuleShardKey("A"), should.ErrLike("does not match"))
				// Other symbols than _ and -.
				assert.Loosely(t, ValidateModuleShardKey("a.b"), should.ErrLike("does not match"))
				assert.Loosely(t, ValidateModuleShardKey("a b"), should.ErrLike("does not match"))
			})
			t.Run(`Too long`, func(t *ftt.Test) {
				assert.Loosely(t, ValidateModuleShardKey(strings.Repeat("a", moduleShardKeyMaxLength+1)), should.ErrLike("must be at most 50 bytes long"))
			})
		})
	})
}
