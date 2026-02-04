// Copyright 2026 The LUCI Authors.
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

package data

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	idspb "go.chromium.org/turboci/proto/go/graph/ids/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/turboci/id"
)

func TestDataSetSet(t *testing.T) {
	t.Parallel()

	rv := func(realm string, value proto.Message) *orchestratorpb.WriteNodesRequest_RealmValue {
		return orchestratorpb.WriteNodesRequest_RealmValue_builder{
			Realm: &realm,
			Value: Value(value),
		}.Build()
	}
	commitRev := &orchestratorpb.Revision{}
	chkId := id.Check("base_check")
	mkId := func(idx int32) *idspb.Identifier {
		return id.Wrap(idspb.CheckOption_builder{
			Check: proto.CloneOf(chkId),
			Idx:   &idx,
		}.Build())
	}
	stdRealm := "project:realm"
	diffRealm := "project:different_realm"

	testCases := []struct {
		name string

		data        []*orchestratorpb.Datum
		wantDataErr string

		set        *orchestratorpb.WriteNodesRequest_RealmValue
		wantSetErr string

		want []*orchestratorpb.Datum
	}{
		{
			name: "add_to_empty",
			set:  rv(stdRealm, &emptypb.Empty{}),
			want: []*orchestratorpb.Datum{
				orchestratorpb.Datum_builder{
					Identifier: mkId(1),
					Realm:      &stdRealm,
					Value:      Value(&emptypb.Empty{}),
					Version:    commitRev,
				}.Build(),
			},
		},
		{
			name: "overwrite",
			data: []*orchestratorpb.Datum{
				orchestratorpb.Datum_builder{
					Identifier: mkId(1),
					Realm:      &stdRealm,
					Value:      Value(structpb.NewStringValue("old")),
				}.Build(),
			},
			set: rv(stdRealm, structpb.NewStringValue("new")),
			want: []*orchestratorpb.Datum{
				orchestratorpb.Datum_builder{
					Identifier: mkId(1),
					Realm:      &stdRealm,
					Value:      Value(structpb.NewStringValue("new")),
					Version:    commitRev,
				}.Build(),
			},
		},
		{
			name: "add_new",
			data: []*orchestratorpb.Datum{
				orchestratorpb.Datum_builder{
					Identifier: mkId(1),
					Realm:      &stdRealm,
					Value:      Value(structpb.NewStringValue("old")),
				}.Build(),
			},
			set: rv(stdRealm, &emptypb.Empty{}),
			want: []*orchestratorpb.Datum{
				orchestratorpb.Datum_builder{
					Realm:      &stdRealm,
					Identifier: mkId(1),
					Value:      Value(structpb.NewStringValue("old")),
				}.Build(),
				orchestratorpb.Datum_builder{
					Realm:      &stdRealm,
					Identifier: mkId(2),
					Value:      Value(&emptypb.Empty{}),
					Version:    commitRev,
				}.Build(),
			},
		},
		{
			name: "mismatched_realm",
			data: []*orchestratorpb.Datum{
				orchestratorpb.Datum_builder{
					Identifier: mkId(1),
					Realm:      &stdRealm,
					Value:      Value(structpb.NewStringValue("old")),
				}.Build(),
			},
			set:        rv(diffRealm, structpb.NewStringValue("new")),
			wantSetErr: "mismatched realm",
		},
		{
			name: "no_realm_write_ok",
			data: []*orchestratorpb.Datum{
				orchestratorpb.Datum_builder{
					Identifier: mkId(1),
					Realm:      &stdRealm,
					Value:      Value(structpb.NewStringValue("old")),
				}.Build(),
			},
			set: rv("", structpb.NewStringValue("new")),
			want: []*orchestratorpb.Datum{
				orchestratorpb.Datum_builder{
					Identifier: mkId(1),
					Realm:      &stdRealm,
					Value:      Value(structpb.NewStringValue("new")),
					Version:    commitRev,
				}.Build(),
			},
		},
		{
			name: "kind mismatched",
			data: []*orchestratorpb.Datum{
				orchestratorpb.Datum_builder{
					Identifier: id.Wrap(id.Stage("bogus")),
					Value:      Value(&emptypb.Empty{}),
				}.Build(),
			},
			wantDataErr: "kind mismatch",
		},
		{
			name: "duplicate",
			data: []*orchestratorpb.Datum{
				orchestratorpb.Datum_builder{
					Identifier: mkId(1),
					Value:      Value(&emptypb.Empty{}),
				}.Build(),
				orchestratorpb.Datum_builder{
					Identifier: mkId(2),
					Value:      Value(&emptypb.Empty{}),
				}.Build(),
			},
			wantDataErr: "duplicate",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, err := CheckOptionDataSetFromSlice(chkId, tc.data)
			if tc.wantDataErr != "" {
				assert.ErrIsLike(t, err, tc.wantDataErr)
				return
			}
			assert.NoErr(t, err)

			err = m.Set(tc.set)
			if tc.wantSetErr != "" {
				assert.ErrIsLike(t, err, tc.wantSetErr)
				return
			}
			assert.NoErr(t, err)

			assert.That(t, m.ToSlice(), should.Match(tc.want))
		})
	}
}
