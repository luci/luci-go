// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package value

import (
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// Tests [filterState.filterRef] by virtue of Stage.Args.
func TestFilterRef(t *testing.T) {
	t.Parallel()

	makeRef := func(t testing.TB, src DataSource, msg proto.Message) *orchestratorpb.ValueRef {
		t.Helper()

		ret := mustInline(msg, "proj:realm")

		if src != nil {
			AbsorbInline(src, ret)
		}

		return ret
	}

	filter, err := ParseFilter(orchestratorpb.ValueFilter_builder{
		TypeInfo: orchestratorpb.TypeInfo_builder{
			// want google.protobuf.*
			Wanted:        TypeSetBuilder{}.WithPackagesOf((*structpb.Value)(nil)).MustBuild(),
			UnknownJsonpb: proto.Bool(true),
			// know google.protobuf.Value
			Known: TypeSetBuilder{}.WithMessages((*structpb.Value)(nil)).MustBuild(),
		}.Build(),

		StageArgs: orchestratorpb.ValueMask_VALUE_MASK_TYPE_VALUE.Enum(),
	}.Build())
	assert.NoErr(t, err)

	accessOK := func(string) (bool, error) { return true, nil }

	t.Run(`want_binary_inline`, func(t *testing.T) {
		stg := orchestratorpb.Stage_builder{
			Args: makeRef(t, nil, structpb.NewBoolValue(true)),
		}.Build()

		rslt, err := FilterStage(stg, filter, accessOK)
		assert.NoErr(t, err)
		assert.Loosely(t, rslt.WantedDigests, should.BeEmpty)
		assert.Loosely(t, rslt.WantedJSON, should.BeEmpty)

		assert.Loosely(t, stg.GetArgs().GetOmitReason(), should.BeZero)
	})

	t.Run(`want_binary_remote`, func(t *testing.T) {
		mSrc := SimpleDataSource{}
		stg := orchestratorpb.Stage_builder{
			Args: makeRef(t, mSrc, structpb.NewBoolValue(true)),
		}.Build()
		dgst := "WoSPrsJSnpq_1pHjFqDjDF6z6la99vAXGBtj_mij-FFaAQ"

		rslt, err := FilterStage(stg, filter, accessOK)
		assert.NoErr(t, err)
		assert.That(t, rslt.WantedDigests, should.Match([]string{dgst}))
		assert.Loosely(t, rslt.WantedJSON, should.BeEmpty)

		assert.That(t, stg.GetArgs().GetDigest(), should.Match(dgst))
	})

	t.Run(`want_json_inline`, func(t *testing.T) {
		lst, err := structpb.NewList([]any{true})
		assert.NoErr(t, err)

		stg := orchestratorpb.Stage_builder{
			Args: makeRef(t, nil, lst),
		}.Build()

		rslt, err := FilterStage(stg, filter, accessOK)
		assert.NoErr(t, err)
		assert.Loosely(t, rslt.WantedDigests, should.BeEmpty)
		assert.That(t, rslt.WantedJSON, should.Match([]*orchestratorpb.ValueRef{
			stg.GetArgs(),
		}))
	})

	t.Run(`want_json_remote`, func(t *testing.T) {
		mSrc := SimpleDataSource{}
		lst, err := structpb.NewList([]any{true})
		assert.NoErr(t, err)

		stg := orchestratorpb.Stage_builder{
			Args: makeRef(t, mSrc, lst),
		}.Build()
		dgst := "PJ5p-94XRaBpvxxK8w4iiX8IWLV7k8O8MICccEIwyhRmAQ"

		rslt, err := FilterStage(stg, filter, accessOK)
		assert.NoErr(t, err)
		assert.That(t, rslt.WantedDigests, should.Match([]string{dgst}))
		assert.That(t, rslt.WantedJSON, should.Match([]*orchestratorpb.ValueRef{
			stg.GetArgs(),
		}))

		assert.That(t, stg.GetArgs().GetDigest(), should.Match(dgst))
	})

	t.Run(`want_no_access`, func(t *testing.T) {
		stg := orchestratorpb.Stage_builder{
			Args: makeRef(t, nil, structpb.NewBoolValue(true)),
		}.Build()

		rslt, err := FilterStage(stg, filter, func(realm string) (bool, error) {
			return false, nil
		})
		assert.NoErr(t, err)
		assert.Loosely(t, rslt.WantedDigests, should.BeEmpty)
		assert.Loosely(t, rslt.WantedJSON, should.BeEmpty)

		assert.That(t, stg.GetArgs().GetOmitReason(), should.Match(
			orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS,
		))
		assert.That(t, stg.GetArgs().HasDigest(), should.BeFalse)
		assert.That(t, stg.GetArgs().HasInline(), should.BeFalse)
	})

	t.Run(`unwant_structural_inline`, func(t *testing.T) {
		filterCopy := &Filter{
			vf: proto.CloneOf(filter.vf),
			ti: filter.ti,
		}
		filterCopy.vf.ClearStageArgs()

		stg := orchestratorpb.Stage_builder{
			Args: makeRef(t, nil, structpb.NewBoolValue(true)),
		}.Build()

		rslt, err := FilterStage(stg, filterCopy, accessOK)
		assert.NoErr(t, err)
		assert.Loosely(t, rslt.WantedDigests, should.BeEmpty)
		assert.Loosely(t, rslt.WantedJSON, should.BeEmpty)

		assert.That(t, stg.GetArgs().GetOmitReason(), should.Match(
			orchestratorpb.OmitReason_OMIT_REASON_UNWANTED,
		))
		assert.That(t, stg.GetArgs().GetDigest(), should.Equal(
			"WoSPrsJSnpq_1pHjFqDjDF6z6la99vAXGBtj_mij-FFaAQ"))
		assert.That(t, stg.GetArgs().HasInline(), should.BeFalse)
	})

	t.Run(`unwant_structural_remote`, func(t *testing.T) {
		filterCopy := &Filter{
			vf: proto.CloneOf(filter.vf),
			ti: filter.ti,
		}
		filterCopy.vf.ClearStageArgs()
		mSrc := SimpleDataSource{}

		stg := orchestratorpb.Stage_builder{
			Args: makeRef(t, mSrc, structpb.NewBoolValue(true)),
		}.Build()

		rslt, err := FilterStage(stg, filterCopy, accessOK)
		assert.NoErr(t, err)
		assert.Loosely(t, rslt.WantedDigests, should.BeEmpty)
		assert.Loosely(t, rslt.WantedJSON, should.BeEmpty)

		assert.That(t, stg.GetArgs().GetOmitReason(), should.Match(
			orchestratorpb.OmitReason_OMIT_REASON_UNWANTED,
		))
		assert.That(t, stg.GetArgs().GetDigest(), should.Equal(
			"WoSPrsJSnpq_1pHjFqDjDF6z6la99vAXGBtj_mij-FFaAQ"))
		assert.That(t, stg.GetArgs().HasInline(), should.BeFalse)
	})

	t.Run(`unwant_type_inline`, func(t *testing.T) {
		stg := orchestratorpb.Stage_builder{
			Args: makeRef(t, nil, structpb.NewBoolValue(true)),
		}.Build()
		stg.GetArgs().SetTypeUrl(TypePrefix + "bogus.namespace.Message")
		stg.GetArgs().GetInline().TypeUrl = TypePrefix + "bogus.namespace.Message"

		rslt, err := FilterStage(stg, filter, accessOK)
		assert.NoErr(t, err)
		assert.Loosely(t, rslt.WantedDigests, should.BeEmpty)
		assert.Loosely(t, rslt.WantedJSON, should.BeEmpty)

		assert.That(t, stg.GetArgs().GetOmitReason(), should.Match(
			orchestratorpb.OmitReason_OMIT_REASON_UNWANTED,
		))
		assert.That(t, stg.GetArgs().GetDigest(), should.Equal(
			"KO4hlqgtld1YeumdZxLIVGmUD1OUfvESdhQVPfDvliFeAQ"))
		assert.That(t, stg.GetArgs().HasInline(), should.BeFalse)
	})

	t.Run(`auth_error`, func(t *testing.T) {
		stg := orchestratorpb.Stage_builder{
			Args: makeRef(t, nil, structpb.NewBoolValue(true)),
		}.Build()

		_, err := FilterStage(stg, filter, func(realm string) (bool, error) {
			return false, errors.New("oh no auth exploded")
		})
		assert.ErrIsLike(t, err, "oh no auth exploded")
	})
}

var valueRefDesc = ((*orchestratorpb.ValueRef)(nil)).ProtoReflect().Descriptor()
var editDesc = (*orchestratorpb.Edit)(nil).ProtoReflect().Descriptor()

func populateAllValueRefs(msg proto.Message) {
	// cached helper to tell if we need to walk down a given branch.
	hasRefCache := make(map[protoreflect.Descriptor]bool, 50)
	var hasValueRef func(desc protoreflect.MessageDescriptor) bool
	hasValueRef = func(desc protoreflect.MessageDescriptor) bool {
		if desc == nil {
			return false
		}
		if desc == valueRefDesc {
			return true
		}

		hasRef, explored := hasRefCache[desc]
		if explored {
			return hasRef
		}
		hasRefCache[desc] = false

		fields := desc.Fields()
		for i := range fields.Len() {
			fd := fields.Get(i)
			msg := fd.Message()
			if msg == nil && fd.IsMap() {
				msg = fd.MapValue().Message()
			}
			if msg != nil && (msg == valueRefDesc || hasValueRef(msg)) {
				hasRefCache[desc] = true
				return true
			}
		}

		return false
	}

	// When processing Edit messages, we want to skip the irrelevant edit delta
	// field.
	editDeltaSkip := "stage"
	if msg.ProtoReflect().Descriptor() == (*orchestratorpb.Stage)(nil).ProtoReflect().Descriptor() {
		editDeltaSkip = "check"
	}

	// Use reflection to generate a skeleton Message with all ValueRef fields set
	// to an instance of a ValueRef without omit reason.
	var skeletonize func(msg protoreflect.Message)
	skeletonize = func(msg protoreflect.Message) {
		if msg.Descriptor() == valueRefDesc {
			proto.Merge(msg.Interface(), orchestratorpb.ValueRef_builder{
				TypeUrl: proto.String(URL[*structpb.Struct]()),
				Digest:  proto.String("something"),
			}.Build())
			return
		}
		isEdit := msg.Descriptor() == editDesc

		fields := msg.Descriptor().Fields()
		for i := range fields.Len() {
			fd := fields.Get(i)
			if isEdit && fd.Name() == protoreflect.Name(editDeltaSkip) {
				continue
			}
			if fd.IsList() {
				if hasValueRef(fd.Message()) {
					skeletonize(msg.Mutable(fd).List().AppendMutable().Message())
				}
			} else if fd.IsMap() {
				if hasValueRef(fd.MapValue().Message()) {
					skeletonize(msg.Mutable(fd).Map().Mutable(fd.MapKey().Default().MapKey()).Message())
				}
			} else if hasValueRef(fd.Message()) {
				skeletonize(msg.Mutable(fd).Message())
			}
		}
	}
	skeletonize(msg.ProtoReflect())
}

func checkAllValueRefsNoAccess(t testing.TB, msg proto.Message) {
	var checkRefs func(field protoreflect.FieldDescriptor, msg protoreflect.Message)
	checkRefs = func(field protoreflect.FieldDescriptor, msg protoreflect.Message) {
		if msg.Descriptor() == valueRefDesc {
			vr := msg.Interface().(*orchestratorpb.ValueRef)
			check.That(t, vr.GetOmitReason(), should.Equal(orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS),
				truth.Explain("in %q", field.FullName()))
			return
		}

		for fd, field := range msg.Range {
			if fd.IsList() {
				checkRefs(fd, field.List().Get(0).Message())
			} else if fd.IsMap() {
				checkRefs(fd, field.Map().Get(fd.MapKey().Default().MapKey()).Message())
			} else {
				checkRefs(fd, field.Message())
			}
		}
	}
	checkRefs(nil, msg.ProtoReflect())
}

// This is a 'change detector test' to ensure that FilterStage touches every
// ValueRef in Stage to ensure that protobuf changes do not accidentally
// cause FilterStage to start missing fields.
//
// This keeps all proto reflection out of the prod/hot path and exclusively
// in the tests.
func TestFilterStageFields(t *testing.T) {
	stg := &orchestratorpb.Stage{}
	populateAllValueRefs(stg)

	rslt, err := FilterStage(stg, nil, func(realm string) (bool, error) { return false, nil })
	assert.NoErr(t, err)
	assert.Loosely(t, rslt.WantedDigests, should.BeEmpty)
	assert.Loosely(t, rslt.WantedJSON, should.BeEmpty)

	checkAllValueRefsNoAccess(t, stg)
}

// This is a 'change detector test' to ensure that FilterStageAttempt touches
// every ValueRef in Stage_Attempt to ensure that protobuf changes do not
// accidentally cause FilterStage to start missing fields.
//
// This keeps all proto reflection out of the prod/hot path and exclusively
// in the tests.
func TestFilterStageAttemptFields(t *testing.T) {
	sa := &orchestratorpb.Stage_Attempt{}
	populateAllValueRefs(sa)

	rslt, err := FilterStageAttempt(sa, nil, func(realm string) (bool, error) { return false, nil })
	assert.NoErr(t, err)
	assert.Loosely(t, rslt.WantedDigests, should.BeEmpty)
	assert.Loosely(t, rslt.WantedJSON, should.BeEmpty)

	checkAllValueRefsNoAccess(t, sa)
}

// This is a 'change detector test' to ensure that FilterCheck touches every
// ValueRef in Check to ensure that protobuf changes do not accidentally
// cause FilterStage to start missing fields.
//
// This keeps all proto reflection out of the prod/hot path and exclusively
// in the tests.
func TestFilterCheckFields(t *testing.T) {
	chk := &orchestratorpb.Check{}
	populateAllValueRefs(chk)

	rslt, err := FilterCheck(chk, nil, func(realm string) (bool, error) { return false, nil })
	assert.NoErr(t, err)
	assert.Loosely(t, rslt.WantedDigests, should.BeEmpty)
	assert.Loosely(t, rslt.WantedJSON, should.BeEmpty)

	checkAllValueRefsNoAccess(t, chk)
}
