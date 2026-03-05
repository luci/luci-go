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
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestOmit(t *testing.T) {
	t.Parallel()

	t.Run(`inline_unwanted`, func(t *testing.T) {
		t.Parallel()

		ref := mustInline(structpb.NewStringValue("hi"), "proj:realm")
		Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_UNWANTED)

		assert.That(t, ref, should.Match(orchestratorpb.ValueRef_builder{
			TypeUrl:    proto.String(URL[*structpb.Value]()),
			Digest:     proto.String("Cn5BE44m4nRbpEEdjyl_nM9jfiUT4jpVUbDo21eSgIleAQ"),
			OmitReason: orchestratorpb.OmitReason_OMIT_REASON_UNWANTED.Enum(),
			Realm:      proto.String("proj:realm"),
		}.Build()))
	})

	t.Run(`inline_noaccess`, func(t *testing.T) {
		t.Parallel()

		ref := mustInline(structpb.NewStringValue("hi"), "proj:realm")
		Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS)

		assert.That(t, ref, should.Match(orchestratorpb.ValueRef_builder{
			TypeUrl:    proto.String(URL[*structpb.Value]()),
			OmitReason: orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS.Enum(),
			Realm:      proto.String("proj:realm"),
		}.Build()))
	})

	t.Run(`outboard_unwanted`, func(t *testing.T) {
		t.Parallel()
		dSrc := SimpleDataSource{}

		ref := mustInline(structpb.NewStringValue("hi"), "proj:realm")
		AbsorbInline(dSrc, ref)

		Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_UNWANTED)

		assert.That(t, ref, should.Match(orchestratorpb.ValueRef_builder{
			TypeUrl:    proto.String(URL[*structpb.Value]()),
			Digest:     proto.String("Cn5BE44m4nRbpEEdjyl_nM9jfiUT4jpVUbDo21eSgIleAQ"),
			OmitReason: orchestratorpb.OmitReason_OMIT_REASON_UNWANTED.Enum().Enum(),
			Realm:      proto.String("proj:realm"),
		}.Build()))
	})

	t.Run(`outboard_noaccess`, func(t *testing.T) {
		t.Parallel()
		dSrc := SimpleDataSource{}

		ref := mustInline(structpb.NewStringValue("hi"), "proj:realm")
		AbsorbInline(dSrc, ref)

		Omit(ref, orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS)

		assert.That(t, ref, should.Match(orchestratorpb.ValueRef_builder{
			TypeUrl:    proto.String(URL[*structpb.Value]()),
			OmitReason: orchestratorpb.OmitReason_OMIT_REASON_NO_ACCESS.Enum(),
			Realm:      proto.String("proj:realm"),
		}.Build()))
	})
}
