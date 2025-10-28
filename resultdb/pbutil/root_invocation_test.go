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

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestRootInvocationID(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateRootInvocationID", t, func(t *ftt.Test) {
		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateRootInvocationID("a"), should.BeNil)
			assert.Loosely(t, ValidateRootInvocationID("a-b_c.d"), should.BeNil)
		})

		t.Run("Invalid", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				assert.Loosely(t, ValidateRootInvocationID(""), should.ErrLike("unspecified"))
			})
			t.Run("Uppercase", func(t *ftt.Test) {
				assert.Loosely(t, ValidateRootInvocationID("A"), should.ErrLike("does not match"))
			})
			t.Run("Starts with dash", func(t *ftt.Test) {
				assert.Loosely(t, ValidateRootInvocationID("-a"), should.ErrLike("does not match"))
			})
			t.Run("Starts with underscore", func(t *ftt.Test) {
				assert.Loosely(t, ValidateRootInvocationID("_a"), should.ErrLike("does not match"))
			})
			t.Run("Starts with dot", func(t *ftt.Test) {
				assert.Loosely(t, ValidateRootInvocationID(".a"), should.ErrLike("does not match"))
			})
			t.Run("Starts with number", func(t *ftt.Test) {
				assert.Loosely(t, ValidateRootInvocationID("1a"), should.ErrLike("does not match"))
			})
			t.Run("Too long", func(t *ftt.Test) {
				assert.Loosely(t, ValidateRootInvocationID(strings.Repeat("a", rootInvocationMaxLength+1)), should.ErrLike("must be at most 100 bytes long"))
			})
		})
	})
}

func TestRootInvocationName(t *testing.T) {
	t.Parallel()
	ftt.Run("(Try)ParseRootInvocationName", t, func(t *ftt.Test) {
		t.Run("Valid", func(t *ftt.Test) {
			testCase := "rootInvocations/u-rootinv"
			rootInvID, err := ParseRootInvocationName(testCase)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rootInvID, should.Equal("u-rootinv"))

			rootInvID, ok := TryParseRootInvocationName(testCase)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, rootInvID, should.Equal("u-rootinv"))
		})

		t.Run("Invalid root invocation ID", func(t *ftt.Test) {
			testCase := "rootInvocations/INVALID_ID"
			_, err := ParseRootInvocationName(testCase)
			assert.Loosely(t, err, should.ErrLike(`does not match`))

			_, ok := TryParseRootInvocationName(testCase)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("Too long root invocation ID", func(t *ftt.Test) {
			longID := strings.Repeat("a", rootInvocationMaxLength+1)
			testCase := "rootInvocations/" + longID
			_, err := ParseRootInvocationName(testCase)
			assert.Loosely(t, err, should.ErrLike(`root invocation ID must be at most 100 bytes long`))

			_, ok := TryParseRootInvocationName(testCase)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("Empty root name", func(t *ftt.Test) {
			testCase := "rootInvocations/"
			_, err := ParseRootInvocationName(testCase)
			assert.Loosely(t, err, should.ErrLike("does not match"))

			_, ok := TryParseRootInvocationName(testCase)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run("Empty", func(t *ftt.Test) {
			testCase := ""
			_, err := ParseRootInvocationName(testCase)
			assert.Loosely(t, err, should.ErrLike("unspecified"))

			_, ok := TryParseRootInvocationName(testCase)
			assert.Loosely(t, ok, should.BeFalse)
		})
	})

	ftt.Run("RootInvocationName", t, func(t *ftt.Test) {
		assert.Loosely(t, RootInvocationName("rootinv"), should.Equal("rootInvocations/rootinv"))
	})

	ftt.Run("ValidateRootInvocationName", t, func(t *ftt.Test) {
		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateRootInvocationName("rootInvocations/a"), should.BeNil)
		})
		t.Run("Invalid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateRootInvocationName("invocations/a"), should.ErrLike("does not match"))
		})
	})

	ftt.Run(`ValidateStreamingExportState`, t, func(t *ftt.Test) {
		t.Run(`Valid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateStreamingExportState(pb.RootInvocation_WAIT_FOR_METADATA), should.BeNil)
		})
		t.Run(`Unspecified`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateStreamingExportState(pb.RootInvocation_STREAMING_EXPORT_STATE_UNSPECIFIED), should.ErrLike("unspecified"))
		})
		t.Run(`Invalid`, func(t *ftt.Test) {
			assert.Loosely(t, ValidateStreamingExportState(pb.RootInvocation_StreamingExportState(999)), should.ErrLike("unknown state 999"))
		})
	})
}
