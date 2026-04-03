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

package id

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
)

func TestWrap(t *testing.T) {
	t.Run("WorkPlan", func(t *testing.T) {
		id := Workplan("wp")
		wrapped := Wrap(id)
		assert.That(t, wrapped.GetWorkPlan(), should.Match(id))
	})

	t.Run("Stage", func(t *testing.T) {
		id := Stage("s")
		wrapped := Wrap(id)
		assert.That(t, wrapped.GetStage(), should.Match(id))
	})

	t.Run("StageAttempt", func(t *testing.T) {
		id := must(StageAttemptErr(StageNotWorknode, "s", 1))
		wrapped := Wrap(id)
		assert.That(t, wrapped.GetStageAttempt(), should.Match(id))
	})

	t.Run("StageEdit", func(t *testing.T) {
		id := must(StageEditErr(StageNotWorknode, "s", time.Unix(1, 0)))
		wrapped := Wrap(id)
		assert.That(t, wrapped.GetStageEdit(), should.Match(id))
	})

	t.Run("Check", func(t *testing.T) {
		id := Check("c")
		wrapped := Wrap(id)
		assert.That(t, wrapped.GetCheck(), should.Match(id))
	})

	t.Run("CheckResult", func(t *testing.T) {
		id := must(CheckResultErr("c", 1))
		wrapped := Wrap(id)
		assert.That(t, wrapped.GetCheckResult(), should.Match(id))
	})

	t.Run("CheckEdit", func(t *testing.T) {
		id := must(CheckEditErr("c", time.Unix(1, 0)))
		wrapped := Wrap(id)
		assert.That(t, wrapped.GetCheckEdit(), should.Match(id))
	})

	t.Run("Identifier", func(t *testing.T) {
		id := Wrap(Check("c"))
		wrapped := Wrap(id)
		assert.That(t, wrapped, should.Match(id))
	})

	t.Run("nil", func(t *testing.T) {
		var id *idspb.Check
		assert.Loosely(t, Wrap(id), should.BeNil)
	})
}

func TestKindOf(t *testing.T) {
	assert.That(t, KindOf(Workplan("wp")), should.Equal(idspb.IdentifierKind_IDENTIFIER_KIND_WORK_PLAN))
	assert.That(t, KindOf(Stage("s")), should.Equal(idspb.IdentifierKind_IDENTIFIER_KIND_STAGE))
	assert.That(t, KindOf(must(StageAttemptErr(StageNotWorknode, "s", 1))), should.Equal(idspb.IdentifierKind_IDENTIFIER_KIND_STAGE_ATTEMPT))
	assert.That(t, KindOf(must(StageEditErr(StageNotWorknode, "s", time.Unix(1, 0)))), should.Equal(idspb.IdentifierKind_IDENTIFIER_KIND_STAGE_EDIT))
	assert.That(t, KindOf(Check("c")), should.Equal(idspb.IdentifierKind_IDENTIFIER_KIND_CHECK))
	assert.That(t, KindOf(must(CheckResultErr("c", 1))), should.Equal(idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_RESULT))
	assert.That(t, KindOf(must(CheckEditErr("c", time.Unix(1, 0)))), should.Equal(idspb.IdentifierKind_IDENTIFIER_KIND_CHECK_EDIT))
}

func TestRoot(t *testing.T) {
	t.Run("WorkPlan", func(t *testing.T) {
		id := Workplan("wp")
		wp, check, stage := Root(id)
		assert.That(t, wp, should.Match(id))
		assert.Loosely(t, check, should.BeNil)
		assert.Loosely(t, stage, should.BeNil)
	})

	t.Run("Stage", func(t *testing.T) {
		id := SetWorkplan(Stage("s"), "wp")
		wp, check, stage := Root(id)
		assert.That(t, wp.GetId(), should.Equal("wp"))
		assert.Loosely(t, check, should.BeNil)
		assert.That(t, stage.GetId(), should.Equal("s"))
	})

	t.Run("StageAttempt", func(t *testing.T) {
		id := SetWorkplan(must(StageAttemptErr(StageNotWorknode, "s", 1)), "wp")
		wp, check, stage := Root(id)
		assert.That(t, wp.GetId(), should.Equal("wp"))
		assert.Loosely(t, check, should.BeNil)
		assert.That(t, stage.GetId(), should.Equal("s"))
	})

	t.Run("StageEdit", func(t *testing.T) {
		id := SetWorkplan(must(StageEditErr(StageNotWorknode, "s", time.Unix(1, 0))), "wp")
		wp, check, stage := Root(id)
		assert.That(t, wp.GetId(), should.Equal("wp"))
		assert.Loosely(t, check, should.BeNil)
		assert.That(t, stage.GetId(), should.Equal("s"))
	})

	t.Run("Check", func(t *testing.T) {
		id := SetWorkplan(Check("c"), "wp")
		wp, check, stage := Root(id)
		assert.That(t, wp.GetId(), should.Equal("wp"))
		assert.That(t, check.GetId(), should.Equal("c"))
		assert.Loosely(t, stage, should.BeNil)
	})

	t.Run("CheckResult", func(t *testing.T) {
		id := SetWorkplan(must(CheckResultErr("c", 1)), "wp")
		wp, check, stage := Root(id)
		assert.That(t, wp.GetId(), should.Equal("wp"))
		assert.That(t, check.GetId(), should.Equal("c"))
		assert.Loosely(t, stage, should.BeNil)
	})

	t.Run("CheckEdit", func(t *testing.T) {
		id := SetWorkplan(must(CheckEditErr("c", time.Unix(1, 0))), "wp")
		wp, check, stage := Root(id)
		assert.That(t, wp.GetId(), should.Equal("wp"))
		assert.That(t, check.GetId(), should.Equal("c"))
		assert.Loosely(t, stage, should.BeNil)
	})

	t.Run("nil", func(t *testing.T) {
		wp, check, stage := Root[*idspb.Check](nil)
		assert.Loosely(t, wp, should.BeNil)
		assert.Loosely(t, check, should.BeNil)
		assert.Loosely(t, stage, should.BeNil)
	})
}

func TestStageRoot(t *testing.T) {
	id := must(StageAttemptErr(StageNotWorknode, "s", 1))
	assert.That(t, StageRoot(id).GetId(), should.Equal("s"))
}

func TestCheckRoot(t *testing.T) {
	id := must(CheckResultErr("c", 1))
	assert.That(t, CheckRoot(id).GetId(), should.Equal("c"))
}

func TestSameRoot(t *testing.T) {
	c1 := Check("c")
	c2, err := CheckEditErr("c", time.Now())
	assert.NoErr(t, err)

	c3 := Check("other")

	assert.That(t, SameRoot(c1, c2), should.BeTrue)
	assert.That(t, SameRoot(c1, c3), should.BeFalse)
	assert.That(t, SameRoot(c1, (*idspb.Check)(nil)), should.BeFalse)

	s1 := Stage("s")
	sa, err := StageAttemptErr(StageNotWorknode, "s", 1)
	assert.NoErr(t, err)

	s1wp := SetWorkplan(Stage("s"), "wp")

	assert.That(t, SameRoot(c1, s1), should.BeFalse)
	assert.That(t, SameRoot(s1, s1wp), should.BeFalse)
	assert.That(t, SameRoot(s1, sa), should.BeTrue)
}

func TestSameWorkplan(t *testing.T) {
	c1 := SetWorkplan(Check("c1"), "wp")
	c2 := SetWorkplan(Check("c2"), "wp")
	c3 := SetWorkplan(Check("c1"), "other")

	assert.That(t, SameWorkPlan(c1, c2), should.BeTrue)
	assert.That(t, SameWorkPlan(c1, c3), should.BeFalse)
	assert.That(t, SameWorkPlan(c1, (*idspb.Check)(nil)), should.BeFalse)
}
