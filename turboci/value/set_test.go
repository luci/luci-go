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

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestSetAddIn(t *testing.T) {
	t.Parallel()

	s, err := structpb.NewStruct(map[string]any{"hello": "world"})
	assertNoErr(t, err)

	toSet := []*orchestratorpb.ValueRef{
		// NOTE: BoolValue and StringValue are the same proto message type.
		MustInline(structpb.NewBoolValue(true), "proj:realm"),
		MustInline(structpb.NewBoolValue(false), "proj:realm"),
		MustInline(structpb.NewStringValue("hello"), "proj:realm"),
		MustInline(s, "proj:realm"),
		MustInline(&emptypb.Empty{}, "proj:realm"),
		MustInline(structpb.NewStringValue("goodbye"), "proj:realm"),
	}

	var set []*orchestratorpb.ValueRef

	for _, ref := range toSet {
		var realmConflict bool
		set, realmConflict = SetByTypeIn(set, ref)
		assertFalse(t, realmConflict)
	}

	assertMatch(t, []*orchestratorpb.ValueRef{
		MustInline(&emptypb.Empty{}, "proj:realm"),
		MustInline(s, "proj:realm"),
		MustInline(structpb.NewStringValue("goodbye"), "proj:realm"),
	}, set)

	set, realmConflict := SetByTypeIn(set, MustInline(structpb.NewBoolValue(false), "other:realm"))
	assertTrue(t, realmConflict)

	assertMatch(t, []*orchestratorpb.ValueRef{
		MustInline(&emptypb.Empty{}, "proj:realm"),
		MustInline(s, "proj:realm"),
		MustInline(structpb.NewStringValue("goodbye"), "proj:realm"),
	}, set)

	set, added := AddByTypeIn(set, MustInline(structpb.NewBoolValue(true), "proj:realm"))
	assertFalse(t, added)

	set, added = AddByTypeIn(set, MustInline(&wrapperspb.BoolValue{Value: true}, "proj:realm"))
	assertTrue(t, added)

	assertMatch(t, []*orchestratorpb.ValueRef{
		MustInline(&wrapperspb.BoolValue{Value: true}, "proj:realm"),
		MustInline(&emptypb.Empty{}, "proj:realm"),
		MustInline(s, "proj:realm"),
		MustInline(structpb.NewStringValue("goodbye"), "proj:realm"),
	}, set)
}
