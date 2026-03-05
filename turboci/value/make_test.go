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
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func mustInline(msg proto.Message, realm string) *orchestratorpb.ValueRef {
	ret, err := Inline(msg, realm)
	if err != nil {
		panic(err)
	}
	return ret
}

func TestInline(t *testing.T) {
	t.Parallel()

	t.Run(`nil`, func(t *testing.T) {
		t.Parallel()

		_, err := Inline(nil, "proj:realm")
		assert.ErrIsLike(t, err, "nil source message")
	})

	t.Run(`ok`, func(t *testing.T) {
		t.Parallel()

		sval := structpb.NewStringValue("hello")
		svalBytes, err := proto.Marshal(sval)
		assert.NoErr(t, err)

		assert.That(t, mustInline(sval, "proj:realm"), should.Match(orchestratorpb.ValueRef_builder{
			TypeUrl: proto.String(URL[*structpb.Value]()),
			Realm:   proto.String("proj:realm"),
			Inline: &anypb.Any{
				TypeUrl: URL[*structpb.Value](),
				Value:   svalBytes,
			},
		}.Build()))
	})

	t.Run(`any`, func(t *testing.T) {
		t.Parallel()

		sval := structpb.NewStringValue("hello")
		svalAny, err := anypb.New(sval)
		assert.NoErr(t, err)

		assert.That(t, mustInline(sval, "proj:realm"), should.Match(orchestratorpb.ValueRef_builder{
			TypeUrl: proto.String(URL[*structpb.Value]()),
			Realm:   proto.String("proj:realm"),
			Inline:  svalAny,
		}.Build()))
	})
}

func TestAbsorbInline(t *testing.T) {
	t.Parallel()

	dSrc := SimpleDataSource{}

	ref := mustInline(structpb.NewStringValue("hello"), "proj:realm")

	assert.That(t, ref.HasInline(), should.BeTrue)

	origBinData := ref.GetInline()

	AbsorbInline(dSrc, ref)

	// ref now contains the digest
	wantDigest := "_tyimwGoRK4aWOh9fCKTjL2xLpa6Hs3W7N1Yfzh21nlkAQ"
	assert.That(t, ref.GetDigest(), should.Equal(wantDigest))

	// dSrc now has the data and it's identical.
	//
	// Note that DataSource avoids copying the data and will return the
	// identical pointer which was in `ref`.
	assert.That(t, dSrc.Retrieve(wantDigest).GetBinary(), should.Equal(origBinData))

	// Absorbing again is a no-op.
	AbsorbInline(dSrc, ref)
}
