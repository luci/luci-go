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

	structpb "github.com/golang/protobuf/ptypes/struct"

	"go.chromium.org/luci/common/proto/google"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestProcessingResult(t *testing.T) {
	t.Parallel()

	res := map[string]string{
		"a": "b",
		"c": "d",
	}

	Convey("Read/Write result works", t, func() {
		p := ProcessingResult{}

		So(p.WriteResult(res), ShouldBeNil)

		out := map[string]string{}
		So(p.ReadResult(&out), ShouldBeNil)
		So(out, ShouldResemble, res)

		st := &structpb.Struct{}
		So(p.ReadResultIntoStruct(st), ShouldBeNil)
		So(st, ShouldResembleProto, &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"a": {Kind: &structpb.Value_StringValue{StringValue: "b"}},
				"c": {Kind: &structpb.Value_StringValue{StringValue: "d"}},
			},
		})
	})

	Convey("Conversion to proto", t, func() {
		Convey("Pending", func() {
			p, err := (&ProcessingResult{ProcID: "zzz"}).Proto()
			So(err, ShouldBeNil)
			So(p, ShouldResembleProto, &api.Processor{
				Id:    "zzz",
				State: api.Processor_PENDING,
			})
		})

		Convey("Success", func() {
			proc := &ProcessingResult{
				ProcID:    "zzz",
				CreatedTs: testTime,
				Success:   true,
			}
			proc.WriteResult(map[string]int{"a": 1})

			p, err := proc.Proto()
			So(err, ShouldBeNil)
			So(p, ShouldResembleProto, &api.Processor{
				Id:         "zzz",
				State:      api.Processor_SUCCEEDED,
				FinishedTs: google.NewTimestamp(testTime),
				Result: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"a": {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					},
				},
			})
		})

		Convey("Failure", func() {
			p, err := (&ProcessingResult{
				ProcID:    "zzz",
				CreatedTs: testTime,
				Error:     "blah",
			}).Proto()
			So(err, ShouldBeNil)
			So(p, ShouldResembleProto, &api.Processor{
				Id:         "zzz",
				State:      api.Processor_FAILED,
				FinishedTs: google.NewTimestamp(testTime),
				Error:      "blah",
			})
		})
	})
}
