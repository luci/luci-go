// Copyright 2018 The LUCI Authors.
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

package model

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestProcessingResult(t *testing.T) {
	t.Parallel()

	res := map[string]string{
		"a": "b",
		"c": "d",
	}

	ftt.Run("Read/Write result works", t, func(t *ftt.Test) {
		p := ProcessingResult{}

		assert.Loosely(t, p.WriteResult(res), should.BeNil)

		out := map[string]string{}
		assert.Loosely(t, p.ReadResult(&out), should.BeNil)
		assert.Loosely(t, out, should.Resemble(res))

		st := &structpb.Struct{}
		assert.Loosely(t, p.ReadResultIntoStruct(st), should.BeNil)
		assert.Loosely(t, st, should.Resemble(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"a": {Kind: &structpb.Value_StringValue{StringValue: "b"}},
				"c": {Kind: &structpb.Value_StringValue{StringValue: "d"}},
			},
		}))
	})

	ftt.Run("Conversion to proto", t, func(t *ftt.Test) {
		t.Run("Pending", func(t *ftt.Test) {
			p, err := (&ProcessingResult{ProcID: "zzz"}).Proto()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, p, should.Resemble(&api.Processor{
				Id:    "zzz",
				State: api.Processor_PENDING,
			}))
		})

		t.Run("Success", func(t *ftt.Test) {
			proc := &ProcessingResult{
				ProcID:    "zzz",
				CreatedTs: testutil.TestTime,
				Success:   true,
			}
			proc.WriteResult(map[string]int{"a": 1})

			p, err := proc.Proto()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, p, should.Resemble(&api.Processor{
				Id:         "zzz",
				State:      api.Processor_SUCCEEDED,
				FinishedTs: timestamppb.New(testutil.TestTime),
				Result: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"a": {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					},
				},
			}))
		})

		t.Run("Failure", func(t *ftt.Test) {
			p, err := (&ProcessingResult{
				ProcID:    "zzz",
				CreatedTs: testutil.TestTime,
				Error:     "blah",
			}).Proto()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, p, should.Resemble(&api.Processor{
				Id:         "zzz",
				State:      api.Processor_FAILED,
				FinishedTs: timestamppb.New(testutil.TestTime),
				Error:      "blah",
			}))
		})
	})
}
