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

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestSetAddIn(t *testing.T) {
	t.Parallel()

	s, err := structpb.NewStruct(map[string]any{"hello": "world"})
	assert.NoErr(t, err)

	toSet := []*orchestratorpb.ValueRef{
		// NOTE: BoolValue and StringValue are the same proto message type.
		mustInline(structpb.NewBoolValue(true), "proj:realm"),
		mustInline(structpb.NewBoolValue(false), "proj:realm"),
		mustInline(structpb.NewStringValue("hello"), "proj:realm"),
		mustInline(s, "proj:realm"),
		mustInline(&emptypb.Empty{}, "proj:realm"),
		mustInline(structpb.NewStringValue("goodbye"), "proj:realm"),
	}

	var set []*orchestratorpb.ValueRef

	for _, ref := range toSet {
		var realmConflict bool
		set, realmConflict = SetByTypeIn(set, ref)
		assert.That(t, realmConflict, should.BeFalse)
	}

	assert.That(t, set, should.Match([]*orchestratorpb.ValueRef{
		mustInline(&emptypb.Empty{}, "proj:realm"),
		mustInline(s, "proj:realm"),
		mustInline(structpb.NewStringValue("goodbye"), "proj:realm"),
	}))

	set, realmConflict := SetByTypeIn(set, mustInline(structpb.NewBoolValue(false), "other:realm"))
	assert.That(t, realmConflict, should.BeTrue)

	assert.That(t, set, should.Match([]*orchestratorpb.ValueRef{
		mustInline(&emptypb.Empty{}, "proj:realm"),
		mustInline(s, "proj:realm"),
		mustInline(structpb.NewStringValue("goodbye"), "proj:realm"),
	}))

	set, added := AddByTypeIn(set, mustInline(structpb.NewBoolValue(true), "proj:realm"))
	assert.That(t, added, should.BeFalse)

	set, added = AddByTypeIn(set, mustInline(&wrapperspb.BoolValue{Value: true}, "proj:realm"))
	assert.That(t, added, should.BeTrue)

	assert.That(t, set, should.Match([]*orchestratorpb.ValueRef{
		mustInline(&wrapperspb.BoolValue{Value: true}, "proj:realm"),
		mustInline(&emptypb.Empty{}, "proj:realm"),
		mustInline(s, "proj:realm"),
		mustInline(structpb.NewStringValue("goodbye"), "proj:realm"),
	}))
}
