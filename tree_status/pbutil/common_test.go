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

package pbutil

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/tree_status/proto/v1"
)

func TestValidation(t *testing.T) {
	ftt.Run("tree Name", t, func(t *ftt.Test) {
		t.Run("tree ID", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				err := ValidateTreeID("fuchsia-stem")
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("must be specified", func(t *ftt.Test) {
				err := ValidateTreeID("")
				assert.Loosely(t, err, should.ErrLike("must be specified"))
			})
			t.Run("must match format", func(t *ftt.Test) {
				err := ValidateTreeID("INVALID")
				assert.Loosely(t, err, should.ErrLike("expected format"))
			})
		})

		t.Run("status ID", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				err := ValidateStatusID(strings.Repeat("0", 32))
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("must be specified", func(t *ftt.Test) {
				err := ValidateStatusID("")
				assert.Loosely(t, err, should.ErrLike("must be specified"))
			})
			t.Run("must match format", func(t *ftt.Test) {
				err := ValidateStatusID("INVALID")
				assert.Loosely(t, err, should.ErrLike("expected format"))
			})
		})

		t.Run("general status", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				err := ValidateGeneralStatus(pb.GeneralState_CLOSED)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("must be specified", func(t *ftt.Test) {
				err := ValidateGeneralStatus(pb.GeneralState_GENERAL_STATE_UNSPECIFIED)
				assert.Loosely(t, err, should.ErrLike("must be specified"))
			})
			t.Run("must match format", func(t *ftt.Test) {
				err := ValidateGeneralStatus(pb.GeneralState(100))
				assert.Loosely(t, err, should.ErrLike("invalid enum value"))
			})
		})

		t.Run("message", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				err := ValidateMessage("my message")
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("must be specified", func(t *ftt.Test) {
				err := ValidateMessage("")
				assert.Loosely(t, err, should.ErrLike("must be specified"))
			})
			t.Run("must not exceed length", func(t *ftt.Test) {
				err := ValidateMessage(strings.Repeat("a", 1025))
				assert.Loosely(t, err, should.ErrLike("longer than 1024 bytes"))
			})
			t.Run("invalid utf-8 string", func(t *ftt.Test) {
				err := ValidateMessage("\xbd")
				assert.Loosely(t, err, should.ErrLike("not a valid utf8 string"))
			})
			t.Run("not in unicode normalized form C", func(t *ftt.Test) {
				err := ValidateMessage("\u0065\u0301")
				assert.Loosely(t, err, should.ErrLike("not in unicode normalized form C"))
			})
			t.Run("non printable rune", func(t *ftt.Test) {
				err := ValidateMessage("non printable rune\u0007")
				assert.Loosely(t, err, should.ErrLike("non-printable rune"))
			})
		})

		t.Run("closing builder name", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				err := ValidateClosingBuilderName("projects/chromium-m100/buckets/ci.shadow/builders/Linux 123")
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("empty closing builder is OK", func(t *ftt.Test) {
				err := ValidateClosingBuilderName("")
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("must match format", func(t *ftt.Test) {
				err := ValidateClosingBuilderName("some name")
				assert.Loosely(t, err, should.ErrLike("expected format"))
			})
		})

		t.Run("project", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				err := ValidateProject("chromium-m100")
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("must be specified", func(t *ftt.Test) {
				err := ValidateProject("")
				assert.Loosely(t, err, should.ErrLike("must be specified"))
			})
			t.Run("must match format", func(t *ftt.Test) {
				err := ValidateProject("some name")
				assert.Loosely(t, err, should.ErrLike("expected format"))
			})
		})

		t.Run("parse tree name", func(t *ftt.Test) {
			t.Run("valid", func(t *ftt.Test) {
				treeID, err := ParseTreeName("trees/chromium")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, treeID, should.Equal("chromium"))
			})
			t.Run("must be specified", func(t *ftt.Test) {
				_, err := ParseTreeName("")
				assert.Loosely(t, err, should.ErrLike("must be specified"))
			})
			t.Run("must match format", func(t *ftt.Test) {
				_, err := ParseTreeName("INVALID")
				assert.Loosely(t, err, should.ErrLike("expected format"))
			})
		})
	})
}
