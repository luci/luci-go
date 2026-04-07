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
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

type NodeType string

const (
	Check NodeType = "Check"
	Stage NodeType = "Stage"
)

var (
	valueRefDesc = ((*orchestratorpb.ValueRef)(nil)).ProtoReflect().Descriptor()
	editDesc     = (*orchestratorpb.Edit)(nil).ProtoReflect().Descriptor()
)

// populateAllValueRefs populates one example of every ValueRef reachable from
// `msg`, returning the number of ValueRefs it wrote.
//
// `nodeType` allows the caller to specify whether `msg` is related to a stage
// or a check, so when working with its Edits we populate only the oneof field
// applicable to that type.
func populateAllValueRefs(msg proto.Message, nodeType NodeType) (numRefs int) {
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

	// When processing Edit messages, we want to skip the delta field that
	// applies to the other type (skip `check` for stage edits, and `stage`
	// for check edits).
	var editDeltaToSkip string
	if nodeType == Check {
		editDeltaToSkip = "stage"
	} else if nodeType == Stage {
		editDeltaToSkip = "check"
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
			numRefs++
			return
		}
		isEdit := msg.Descriptor() == editDesc

		fields := msg.Descriptor().Fields()
		for i := range fields.Len() {
			fd := fields.Get(i)
			if isEdit && fd.Name() == protoreflect.Name(editDeltaToSkip) {
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
	return numRefs
}

// TestRefsInStage ensures that `RefsInStage` actually covers all ValueRefs in
// the Stage proto.
//
// If this test fails, it likely means that protobufs for orchestrator.Stage
// have changed and RefsInStage needs to be updated.
func TestRefsInStage(t *testing.T) {
	stg := &orchestratorpb.Stage{}
	expect := populateAllValueRefs(stg, Stage)

	var found int
	for range RefsInStage(stg) {
		found++
	}

	assert.That(t, found, should.Equal(expect))
}

// TestRefsInStageAttempt ensures that `RefsInStageAttempt` actually covers all
// ValueRefs in the Stage Attempt proto.
//
// If this test fails, it likely means that protobufs for
// orchestrator.Stage_Attempt have changed and RefsInStageAttempt needs to be
// updated.
func TestRefsInStageAttempt(t *testing.T) {
	atmpt := &orchestratorpb.Stage_Attempt{}
	expect := populateAllValueRefs(atmpt, Stage)

	var found int
	for range RefsInStageAttempt(atmpt) {
		found++
	}

	assert.That(t, found, should.Equal(expect))
}

// TestRefsInStageEdit ensures that `RefsInStageEdit` actually covers all
// ValueRefs in the Stage Edit proto.
//
// If this test fails, it likely means that protobufs for
// orchestrator.Edit have changed and RefsInStageEdit needs to be updated.
func TestRefsInStageEdit(t *testing.T) {
	edit := orchestratorpb.Edit_builder{
		Stage: &orchestratorpb.StageDelta{},
	}.Build()
	expect := populateAllValueRefs(edit, Stage)

	var found int
	for range RefsInStageEdit(edit) {
		found++
	}

	assert.That(t, found, should.Equal(expect))
}

// TestRefsInCheck ensures that `RefsInCheck` actually covers all ValueRefs in
// the Check proto.
//
// If this test fails, it likely means that protobufs for
// orchestrator.Check have changed and RefsInCheck needs to be updated.
func TestRefsInCheck(t *testing.T) {
	check := &orchestratorpb.Check{}
	expect := populateAllValueRefs(check, Check)

	var found int
	for range RefsInCheck(check) {
		found++
	}

	assert.That(t, found, should.Equal(expect))
}

// TestRefsInCheckEdit ensures that `RefsInCheckEdit` actually covers all
// ValueRefs in the Check Edit proto.
//
// If this test fails, it likely means that protobufs for
// orchestrator.Edit have changed and RefsInCheckEdit needs to be updated.
func TestRefsInCheckEdit(t *testing.T) {
	edit := orchestratorpb.Edit_builder{
		Check: orchestratorpb.CheckDelta_builder{
			State: orchestratorpb.CheckState_CHECK_STATE_FINAL.Enum(),
		}.Build(),
	}.Build()
	expect := populateAllValueRefs(edit, Check)

	var found int
	for range RefsInCheckEdit(edit) {
		found++
	}

	assert.That(t, found, should.Equal(expect))
}
