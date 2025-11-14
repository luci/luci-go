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
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestCheckErr(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		cid, err := CheckErr("hello")
		assert.NoErr(t, err)
		assert.That(t, cid, should.Match(idspb.Check_builder{
			Id: proto.String("hello"),
		}.Build()))
	})

	t.Run("bad", func(t *testing.T) {
		_, err := CheckErr("hello:world")
		assert.ErrIsLike(t, err, `id.Check: id: "hello:world" contains ":"`)
	})
}

func TestCheckOptionErr(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		id, err := CheckOptionErr("h", 1)
		assert.NoErr(t, err)
		assert.That(t, id, should.Match(
			idspb.CheckOption_builder{
				Check: idspb.Check_builder{Id: proto.String("h")}.Build(),
				Idx:   proto.Int32(1),
			}.Build(),
		))
	})

	t.Run("bad checkid", func(t *testing.T) {
		_, err := CheckOptionErr("h:", 1)
		assert.ErrIsLike(t, err, `id.CheckOption: id.Check: id: "h:" contains ":"`)
	})

	t.Run("bad idx", func(t *testing.T) {
		_, err := CheckOptionErr("h", 0)
		assert.ErrIsLike(t, err, `id.CheckOption: optionIdx: 0 must be in [1, max(int32)]`)
	})
}

func TestCheckResultErr(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		id, err := CheckResultErr("h", 1)
		assert.NoErr(t, err)
		assert.That(t, id, should.Match(
			idspb.CheckResult_builder{
				Check: idspb.Check_builder{Id: proto.String("h")}.Build(),
				Idx:   proto.Int32(1),
			}.Build(),
		))
	})

	t.Run("bad checkid", func(t *testing.T) {
		_, err := CheckResultErr("h:", 1)
		assert.ErrIsLike(t, err, `id.CheckResult: id.Check: id: "h:" contains ":"`)
	})

	t.Run("bad idx", func(t *testing.T) {
		_, err := CheckResultErr("h", 0)
		assert.ErrIsLike(t, err, `id.CheckResult: resultIdx: 0 must be in [1, max(int32)]`)
	})
}

func TestCheckResultDatumErr(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		id, err := CheckResultDatumErr("h", 1, 2)
		assert.NoErr(t, err)
		assert.That(t, id, should.Match(
			idspb.CheckResultDatum_builder{
				Result: idspb.CheckResult_builder{
					Check: idspb.Check_builder{Id: proto.String("h")}.Build(),
					Idx:   proto.Int32(1),
				}.Build(),
				Idx: proto.Int32(2),
			}.Build(),
		))
	})

	t.Run("bad checkid", func(t *testing.T) {
		_, err := CheckResultDatumErr("h:", 1, 2)
		assert.ErrIsLike(t, err, `id.CheckResultDatum: id.CheckResult: id.Check: id: "h:" contains ":"`)
	})

	t.Run("bad result idx", func(t *testing.T) {
		_, err := CheckResultDatumErr("h", 0, 2)
		assert.ErrIsLike(t, err, `id.CheckResultDatum: id.CheckResult: resultIdx: 0 must be in [1, max(int32)]`)
	})

	t.Run("bad datum idx", func(t *testing.T) {
		_, err := CheckResultDatumErr("h", 1, 0)
		assert.ErrIsLike(t, err, `id.CheckResultDatum: datumIdx: 0 must be in [1, max(int32)]`)
	})
}

func TestCheckEditErr(t *testing.T) {
	ts := time.Unix(12345, 6789)

	t.Run("ok", func(t *testing.T) {
		id, err := CheckEditErr("h", ts)
		assert.NoErr(t, err)
		assert.That(t, id, should.Match(
			idspb.CheckEdit_builder{
				Check:   idspb.Check_builder{Id: proto.String("h")}.Build(),
				Version: timestamppb.New(ts),
			}.Build(),
		))
	})

	t.Run("bad checkid", func(t *testing.T) {
		_, err := CheckEditErr("h:", ts)
		assert.ErrIsLike(t, err, `id.CheckEdit: id.Check: id: "h:" contains ":"`)
	})

	t.Run("bad ts", func(t *testing.T) {
		_, err := CheckEditErr("h", time.Time{})
		assert.ErrIsLike(t, err, `id.CheckEdit: zero timestamp`)
	})
}

func TestCheckEditOptionErr(t *testing.T) {
	ts := time.Unix(12345, 6789)

	t.Run("ok", func(t *testing.T) {
		id, err := CheckEditOptionErr("h", ts, 1)
		assert.NoErr(t, err)
		assert.That(t, id, should.Match(
			idspb.CheckEditOption_builder{
				CheckEdit: idspb.CheckEdit_builder{
					Check:   idspb.Check_builder{Id: proto.String("h")}.Build(),
					Version: timestamppb.New(ts),
				}.Build(),
				Idx: proto.Int32(1),
			}.Build(),
		))
	})

	t.Run("bad checkid", func(t *testing.T) {
		_, err := CheckEditOptionErr("h:", ts, 1)
		assert.ErrIsLike(t, err, `id.CheckEditOption: id.CheckEdit: id.Check: id: "h:" contains ":"`)
	})

	t.Run("bad ts", func(t *testing.T) {
		_, err := CheckEditOptionErr("h", time.Time{}, 1)
		assert.ErrIsLike(t, err, `id.CheckEditOption: id.CheckEdit: zero timestamp`)
	})

	t.Run("bad idx", func(t *testing.T) {
		_, err := CheckEditOptionErr("h", ts, 0)
		assert.ErrIsLike(t, err, `id.CheckEditOption: optionIdx: 0 must be in [1, max(int32)]`)
	})
}

func TestStageErr(t *testing.T) {
	t.Run("ok S", func(t *testing.T) {
		id, err := StageErr(StageNotWorknode, "something")
		assert.NoErr(t, err)
		assert.That(t, id, should.Match(
			idspb.Stage_builder{Id: proto.String("something"), IsWorknode: proto.Bool(false)}.Build(),
		))
	})
	t.Run("ok N", func(t *testing.T) {
		id, err := StageErr(StageIsWorknode, "something")
		assert.NoErr(t, err)
		assert.That(t, id, should.Match(
			idspb.Stage_builder{Id: proto.String("something"), IsWorknode: proto.Bool(true)}.Build(),
		))
	})

	t.Run("empty prefix", func(t *testing.T) {
		_, err := StageErr(StageIsUnknown, "")
		assert.ErrIsLike(t, err, `id.Stage: stageID: zero length`)
	})

	t.Run("bad format", func(t *testing.T) {
		_, err := StageErr(StageIsUnknown, "some:thing")
		assert.ErrIsLike(t, err, `id.Stage: stageID: "some:thing" contains ":"`)
	})
}

func TestStageAttemptErr(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		id, err := StageAttemptErr(StageNotWorknode, "something", 1)
		assert.NoErr(t, err)
		assert.That(t, id, should.Match(
			idspb.StageAttempt_builder{
				Stage: idspb.Stage_builder{Id: proto.String("something"), IsWorknode: proto.Bool(false)}.Build(),
				Idx:   proto.Int32(1),
			}.Build(),
		))
	})

	t.Run("bad idx", func(t *testing.T) {
		_, err := StageAttemptErr(StageNotWorknode, "something", 0)
		assert.ErrIsLike(t, err, `id.StageAttempt: attemptIdx: 0 must be in [1, max(int32)]`)
	})
}

func TestStageEditErr(t *testing.T) {
	ts := time.Unix(12345, 6789)

	t.Run("ok", func(t *testing.T) {
		id, err := StageEditErr(StageNotWorknode, "something", ts)
		assert.NoErr(t, err)
		assert.That(t, id, should.Match(
			idspb.StageEdit_builder{
				Stage: idspb.Stage_builder{
					Id:         proto.String("something"),
					IsWorknode: proto.Bool(false),
				}.Build(),
				Version: timestamppb.New(ts),
			}.Build(),
		))
	})

	t.Run("bad ts", func(t *testing.T) {
		_, err := StageEditErr(StageNotWorknode, "something", time.Time{})
		assert.ErrIsLike(t, err, `id.StageEdit: zero timestamp`)
	})
}

func TestSetWorkplanErr(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		id, err := CheckErr("c")
		assert.NoErr(t, err)
		id, err = SetWorkplanErr(id, "Lwp")
		assert.NoErr(t, err)
		assert.That(t, id, should.Match(
			idspb.Check_builder{
				WorkPlan: idspb.WorkPlan_builder{Id: proto.String("Lwp")}.Build(),
				Id:       proto.String("c"),
			}.Build(),
		))
	})

	t.Run("bad workplan", func(t *testing.T) {
		id, err := CheckErr("c")
		assert.NoErr(t, err)
		_, err = SetWorkplanErr(id, "Lw:p")
		assert.ErrIsLike(t, err, `id.SetWorkplan: workPlanID: "Lw:p" contains ":"`)
	})
}

func TestSetWorkplan(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		id := SetWorkplan(must(CheckErr("c")), "Lwp")
		assert.That(t, id, should.Match(
			idspb.Check_builder{
				WorkPlan: idspb.WorkPlan_builder{Id: proto.String("Lwp")}.Build(),
				Id:       proto.String("c"),
			}.Build(),
		))
	})

	t.Run("panic", func(t *testing.T) {
		assert.That(t, func() {
			SetWorkplan(must(CheckErr("c")), "Lw:p")
		}, should.PanicLikeString(`contains ":"`))
	})
}

func TestStage(t *testing.T) {
	t.Run(`ok`, func(t *testing.T) {
		sid := Stage("hello")
		assert.That(t, sid, should.Match(idspb.Stage_builder{
			Id:         proto.String("hello"),
			IsWorknode: proto.Bool(false),
		}.Build()))
	})

	t.Run("panic", func(t *testing.T) {
		assert.That(t, func() {
			Stage("")
		}, should.PanicLikeString(`zero length`))
	})
}

func TestWrap(t *testing.T) {
	t.Run("Check", func(t *testing.T) {
		id := must(CheckErr("c"))
		wrapped := Wrap(id)
		assert.That(t, wrapped, should.Match(
			idspb.Identifier_builder{Check: id}.Build(),
		))
		assert.That(t, Wrap(wrapped), should.Equal(wrapped))
	})

	t.Run("WorkPlan", func(t *testing.T) {
		id := idspb.WorkPlan_builder{Id: proto.String("Lwp")}.Build()
		wrapped := Wrap(id)
		assert.That(t, wrapped, should.Match(
			idspb.Identifier_builder{WorkPlan: id}.Build(),
		))
	})
}

func ExampleSetWorkplan() {
	id := SetWorkplan(Check("my-check"), "my-workplan")
	fmt.Println(ToString(id))
	// Output: Lmy-workplan:Cmy-check
}

func ExampleSetWorkplan_identifier() {
	id := SetWorkplan(Wrap(Check("my-check")), "my-workplan")
	fmt.Println(ToString(id))
	// Output: Lmy-workplan:Cmy-check
}

func ExampleCheck() {
	id := Check("my-check")
	fmt.Println(ToString(id))
	// Output: :Cmy-check
}

func ExampleStage() {
	id := Stage("my-stage")
	fmt.Println(ToString(id))
	// Output: :Smy-stage
}

func ExampleStageUnknown() {
	id := StageUnknown("my-stage")
	fmt.Println(ToString(id))
	// Output: :?my-stage
}

func ExampleStageWorkNode() {
	id := StageWorkNode("my-stage")
	fmt.Println(ToString(id))
	// Output: :Nmy-stage
}

func ExampleWrap() {
	cid := Check("my-check")
	id := Wrap(cid)
	fmt.Printf("%T --> %s\n", cid, ToString(cid))
	fmt.Printf("%T --> %s\n", id, ToString(id))
	// Output:
	// *idspb.Check --> :Cmy-check
	// *idspb.Identifier --> :Cmy-check
}
