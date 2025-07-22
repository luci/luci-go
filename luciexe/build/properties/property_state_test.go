// Copyright 2024 The LUCI Authors.
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

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func mustStruct(d map[string]any) *structpb.Struct {
	ret, err := structpb.NewStruct(d)
	if err != nil {
		panic(err)
	}
	return ret
}

func mustJSON(j string) *structpb.Struct {
	ret := &structpb.Struct{}
	if err := protojson.Unmarshal([]byte(j), ret); err != nil {
		panic(err)
	}
	return ret
}

func mctx() (context.Context, *memlogger.MemLogger) {
	ret := memlogger.Use(context.Background())
	return ret, logging.Get(ret).(*memlogger.MemLogger)
}

func TestPropertyStateProto(t *testing.T) {
	t.Parallel()

	t.Run(`internal`, func(t *testing.T) {
		t.Parallel()

		t.Run(`toStruct`, func(t *testing.T) {
			t.Parallel()

			t.Run(`regular names`, func(t *testing.T) {
				t.Parallel()

				psp := &outputPropertyState{
					data: &buildbucketpb.Build{
						Id: 12345,
						BuilderInfo: &buildbucketpb.Build_BuilderInfo{
							Description: "this is a test",
						},
						CanceledBy: "example user",
					},
					toJSON: protoToJSON(true),
				}

				assert.That(t, psp.toStruct(), should.Match(mustStruct(map[string]any{
					"id": "12345",
					"builderInfo": map[string]any{
						"description": "this is a test",
					},
					"canceledBy": "example user",
				})))
			})

			t.Run(`proto names`, func(t *testing.T) {
				t.Parallel()

				psp := &outputPropertyState{
					data: &buildbucketpb.Build{
						Id: 12345,
						BuilderInfo: &buildbucketpb.Build_BuilderInfo{
							Description: "this is a test",
						},
						CanceledBy: "example user",
					},
					toJSON: protoToJSON(false),
				}

				assert.That(t, psp.toStruct(), should.Match(mustStruct(map[string]any{
					"id": "12345",
					"builder_info": map[string]any{
						"description": "this is a test",
					},
					"canceled_by": "example user",
				})))
			})
		})
	})
}

func TestRegisteredProto(t *testing.T) {
	t.Parallel()

	r := Registry{}
	rp := MustRegister[*buildbucketpb.Build](&r, "")
	ctx := context.Background()

	t.Run(`Initial`, func(t *testing.T) {
		t.Parallel()

		s, err := r.Instantiate(ctx, mustStruct(map[string]any{
			"id": 12345,
		}), nil)
		assert.NoErr(t, err)
		assert.That(t, rp.GetInputFromState(s), should.Match(&buildbucketpb.Build{Id: 12345}))
	})

	t.Run(`InitialCtx`, func(t *testing.T) {
		t.Parallel()

		s, err := r.Instantiate(ctx, mustStruct(map[string]any{
			"id": 12345,
		}), nil)
		assert.NoErr(t, err)

		ctx, err := s.SetInContext(context.Background())
		assert.NoErr(t, err)

		assert.That(t, rp.GetInput(ctx), should.Match(&buildbucketpb.Build{Id: 12345}))
	})

	t.Run(`Interact`, func(t *testing.T) {
		t.Parallel()

		s, err := r.Instantiate(ctx, nil, nil)

		assert.NoErr(t, err)

		rp.MutateOutputFromState(s, func(b *buildbucketpb.Build) (mutated bool) {
			assert.That(t, b.Id, should.Equal[int64](0))
			return false
		})

		rp.MutateOutputFromState(s, func(b *buildbucketpb.Build) (mutated bool) {
			b.Id = 12345
			return true
		})

		rp.MutateOutputFromState(s, func(b *buildbucketpb.Build) (mutated bool) {
			assert.That(t, b.Id, should.Equal[int64](12345))
			return false
		})
	})

	t.Run(`InteractCtx`, func(t *testing.T) {
		t.Parallel()

		s, err := r.Instantiate(ctx, nil, nil)
		assert.NoErr(t, err)

		ctx, err := s.SetInContext(context.Background())
		assert.NoErr(t, err)

		rp.MutateOutput(ctx, func(b *buildbucketpb.Build) (mutated bool) {
			assert.That(t, b.Id, should.Equal[int64](0))
			return false
		})

		rp.MutateOutput(ctx, func(b *buildbucketpb.Build) (mutated bool) {
			b.Id = 12345
			return true
		})

		rp.MutateOutput(ctx, func(b *buildbucketpb.Build) (mutated bool) {
			assert.That(t, b.Id, should.Equal[int64](12345))
			return false
		})
	})
}

func TestRegisteredStruct(t *testing.T) {
	t.Parallel()

	r := Registry{}
	ctx := context.Background()

	type myCustomStruct struct {
		Field int
	}

	rp := MustRegister[*myCustomStruct](&r, "")

	t.Run(`Initial`, func(t *testing.T) {
		t.Parallel()

		s, err := r.Instantiate(ctx, mustStruct(map[string]any{
			"Field": 12345,
		}), nil)
		assert.NoErr(t, err)
		assert.That(t, rp.GetInputFromState(s), should.Match(&myCustomStruct{Field: 12345}))
	})

	t.Run(`InitialCtx`, func(t *testing.T) {
		t.Parallel()

		s, err := r.Instantiate(ctx, mustStruct(map[string]any{
			"Field": 12345,
		}), nil)
		assert.NoErr(t, err)

		ctx, err := s.SetInContext(context.Background())
		assert.NoErr(t, err)

		assert.That(t, rp.GetInput(ctx), should.Match(&myCustomStruct{Field: 12345}))
	})

	t.Run(`OutputValue`, func(t *testing.T) {
		t.Parallel()

		s, err := r.Instantiate(ctx, nil, nil)

		assert.NoErr(t, err)

		rp.MutateOutputFromState(s, func(b *myCustomStruct) (mutated bool) {
			assert.That(t, b.Field, should.Equal(0))
			return false
		})

		rp.MutateOutputFromState(s, func(b *myCustomStruct) (mutated bool) {
			b.Field = 12345
			return true
		})

		rp.MutateOutputFromState(s, func(b *myCustomStruct) (mutated bool) {
			assert.That(t, b.Field, should.Equal(12345))
			return false
		})
	})

	t.Run(`OutputValueCtx`, func(t *testing.T) {
		t.Parallel()

		s, err := r.Instantiate(ctx, nil, nil)
		assert.NoErr(t, err)

		ctx, err := s.SetInContext(context.Background())
		assert.That(t, err, should.ErrLike(nil))

		rp.MutateOutput(ctx, func(b *myCustomStruct) (mutated bool) {
			assert.That(t, b.Field, should.Equal(0))
			return false
		})

		rp.MutateOutput(ctx, func(b *myCustomStruct) (mutated bool) {
			b.Field = 12345
			return true
		})

		rp.MutateOutput(ctx, func(b *myCustomStruct) (mutated bool) {
			assert.That(t, b.Field, should.Equal(12345))
			return false
		})
	})
}
