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
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestWriteMatchesRef(t *testing.T) {
	t.Parallel()

	data1, _ := anypb.New(&emptypb.Empty{})
	data2, _ := anypb.New(&structpb.Struct{})

	t.Run(`match inline`, func(t *testing.T) {
		t.Parallel()
		write := orchestratorpb.ValueWrite_builder{
			Realm: proto.String("realm"),
			Data:  data1,
		}.Build()
		ref := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data1,
		}.Build()
		assert.That(t, WriteMatchesRef(write, ref), should.BeTrue)
	})

	t.Run(`match digest`, func(t *testing.T) {
		t.Parallel()
		write := orchestratorpb.ValueWrite_builder{
			Realm: proto.String("realm"),
			Data:  data1,
		}.Build()
		ref := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Digest:  proto.String(string(ComputeDigest(data1))),
		}.Build()
		assert.That(t, WriteMatchesRef(write, ref), should.BeTrue)
	})

	t.Run(`mismatch realm`, func(t *testing.T) {
		t.Parallel()
		write := orchestratorpb.ValueWrite_builder{
			Realm: proto.String("realm1"),
			Data:  data1,
		}.Build()
		ref := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm2"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data1,
		}.Build()
		assert.That(t, WriteMatchesRef(write, ref), should.BeFalse)
	})

	t.Run(`mismatch type url`, func(t *testing.T) {
		t.Parallel()
		write := orchestratorpb.ValueWrite_builder{
			Realm: proto.String("realm"),
			Data:  data1,
		}.Build()
		ref := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data2.TypeUrl),
			Inline:  data1,
		}.Build()
		assert.That(t, WriteMatchesRef(write, ref), should.BeFalse)
	})

	t.Run(`mismatch inline data`, func(t *testing.T) {
		t.Parallel()
		write := orchestratorpb.ValueWrite_builder{
			Realm: proto.String("realm"),
			Data:  data1,
		}.Build()
		ref := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data2,
		}.Build()
		assert.That(t, WriteMatchesRef(write, ref), should.BeFalse)
	})

	t.Run(`mismatch digest`, func(t *testing.T) {
		t.Parallel()
		write := orchestratorpb.ValueWrite_builder{
			Realm: proto.String("realm"),
			Data:  data1,
		}.Build()
		ref := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Digest:  proto.String(string(ComputeDigest(data2))),
		}.Build()
		assert.That(t, WriteMatchesRef(write, ref), should.BeFalse)
	})
}

func TestRefMatchesRef(t *testing.T) {
	t.Parallel()

	data1, _ := anypb.New(&emptypb.Empty{})
	data2, _ := anypb.New(&structpb.Struct{})
	digest1 := string(ComputeDigest(data1))
	digest2 := string(ComputeDigest(data2))

	t.Run(`match inline-inline`, func(t *testing.T) {
		t.Parallel()
		a := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data1,
		}.Build()
		b := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data1,
		}.Build()
		assert.That(t, RefMatchesRef(a, b), should.BeTrue)
	})

	t.Run(`match inline-digest`, func(t *testing.T) {
		t.Parallel()
		a := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data1,
		}.Build()
		b := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Digest:  proto.String(digest1),
		}.Build()
		assert.That(t, RefMatchesRef(a, b), should.BeTrue)
	})

	t.Run(`match digest-inline`, func(t *testing.T) {
		t.Parallel()
		a := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Digest:  proto.String(digest1),
		}.Build()
		b := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data1,
		}.Build()
		assert.That(t, RefMatchesRef(a, b), should.BeTrue)
	})

	t.Run(`match digest-digest`, func(t *testing.T) {
		t.Parallel()
		a := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Digest:  proto.String(digest1),
		}.Build()
		b := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Digest:  proto.String(digest1),
		}.Build()
		assert.That(t, RefMatchesRef(a, b), should.BeTrue)
	})

	t.Run(`mismatch realm`, func(t *testing.T) {
		t.Parallel()
		a := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm1"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data1,
		}.Build()
		b := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm2"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data1,
		}.Build()
		assert.That(t, RefMatchesRef(a, b), should.BeFalse)
	})

	t.Run(`mismatch type url`, func(t *testing.T) {
		t.Parallel()
		a := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String("type1"),
			Inline:  data1,
		}.Build()
		b := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String("type2"),
			Inline:  data1,
		}.Build()
		assert.That(t, RefMatchesRef(a, b), should.BeFalse)
	})

	t.Run(`mismatch inline data`, func(t *testing.T) {
		t.Parallel()
		a := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data1,
		}.Build()
		b := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data2,
		}.Build()
		assert.That(t, RefMatchesRef(a, b), should.BeFalse)
	})

	t.Run(`mismatch digest`, func(t *testing.T) {
		t.Parallel()
		a := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Digest:  proto.String(digest1),
		}.Build()
		b := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Digest:  proto.String(digest2),
		}.Build()
		assert.That(t, RefMatchesRef(a, b), should.BeFalse)
	})

	t.Run(`one missing content`, func(t *testing.T) {
		t.Parallel()
		a := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
			Inline:  data1,
		}.Build()
		b := orchestratorpb.ValueRef_builder{
			Realm:   proto.String("realm"),
			TypeUrl: proto.String(data1.TypeUrl),
		}.Build()
		assert.That(t, RefMatchesRef(a, b), should.BeFalse)
	})
}
