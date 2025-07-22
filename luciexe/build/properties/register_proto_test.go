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

package properties

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestProtoFromStruct(t *testing.T) {
	t.Parallel()

	t.Run(`strict`, func(t *testing.T) {
		t.Parallel()

		target := &buildbucketpb.Build{}
		ctx, ml := mctx()

		badExtras, err := protoFromStruct(ctx, "", rejectUnknownFields, mustStruct(map[string]any{
			"id": 1234,
		}), target)

		assert.That(t, badExtras, should.BeFalse)
		assert.NoErr(t, err)
		assert.That(t, target, should.Match(&buildbucketpb.Build{
			Id: 1234,
		}))

		badExtras, err = protoFromStruct(ctx, "", rejectUnknownFields, mustStruct(map[string]any{
			"morple": 100,
			"id":     1234,
		}), target)
		assert.That(t, badExtras, should.BeTrue)
		assert.NoErr(t, err)
		assert.That(t, ml.Messages(), should.Match([]memlogger.LogEntry{
			{Level: logging.Error, Msg: `Unknown fields while parsing property namespace "": {"morple":100}`, CallDepth: 2},
		}))
	})

	t.Run(`ignore unknown`, func(t *testing.T) {
		t.Parallel()

		target := &buildbucketpb.Build{}

		badExtras, err := protoFromStruct(context.Background(), "", logUnknownFields, mustStruct(map[string]any{
			"morple": 100,
			"id":     1234,
		}), target)
		assert.That(t, badExtras, should.BeFalse)
		assert.NoErr(t, err)
		assert.That(t, target, should.Match(&buildbucketpb.Build{
			Id: 1234,
		}))
	})
}

func TestStructPBPassthrough(t *testing.T) {
	t.Parallel()

	r := Registry{}
	outer := MustRegister[*structpb.Struct](&r, "")
	sub := MustRegister[*structpb.Struct](&r, "$sub")

	rawStruct := mustStruct(map[string]any{
		"random": 100,
		"$sub": map[string]any{
			"some":    []any{"value"},
			"another": "key",
		},
	})

	state, err := r.Instantiate(context.Background(), rawStruct, nil)
	assert.NoErr(t, err)

	// should.Equal makes sure that the original object was passed through.
	assert.That(t, outer.GetInputFromState(state).Fields["random"].GetNumberValue(),
		should.Equal(rawStruct.Fields["random"].GetNumberValue()))
	assert.That(t, sub.GetInputFromState(state),
		should.Equal(rawStruct.Fields["$sub"].GetStructValue()))

	// Output values start empty but non-nil.
	outer.MutateOutputFromState(state, func(s *structpb.Struct) (mutated bool) {
		assert.Loosely(t, s.Fields, should.HaveLength(0))
		return false
	})
	sub.MutateOutputFromState(state, func(s *structpb.Struct) (mutated bool) {
		assert.Loosely(t, s.Fields, should.HaveLength(0))
		return false
	})

	outer.MutateOutputFromState(state, func(s *structpb.Struct) (mutated bool) {
		s.Fields = map[string]*structpb.Value{
			"random": structpb.NewNumberValue(100),
		}
		return true
	})
	sub.MutateOutputFromState(state, func(s *structpb.Struct) (mutated bool) {
		s.Fields = rawStruct.Fields["$sub"].GetStructValue().Fields
		return true
	})

	out, _, _, err := state.Serialize()
	assert.NoErr(t, err)
	assert.That(t, out, should.Match(rawStruct))

	// Serialize always returns a NEW Struct - ensure that these values do NOT
	// equal each other, even though they Match.
	assert.That(t, out.Fields["$sub"].GetStructValue().Fields["some"],
		should.NotEqual(rawStruct.Fields["$sub"].GetStructValue().Fields["some"]))
	assert.That(t, out.Fields["$sub"].GetStructValue().Fields["another"],
		should.NotEqual(rawStruct.Fields["$sub"].GetStructValue().Fields["another"]))
}
