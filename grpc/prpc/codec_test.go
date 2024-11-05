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

package prpc

import (
	"bytes"
	"fmt"
	"testing"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/grpc/prpc/internal/testpb"
)

func TestCodecRoundTripSimple(t *testing.T) {
	t.Parallel()

	msg := &testpb.HelloRequest{Name: "Hi"}

	runCodecRoundTripTests(t, msg, []codecTestCase{
		{codecWireV1, codecWireV1, false, false},
		{codecWireV2, codecWireV2, false, false},
		{codecWireV1, codecWireV2, false, false},
		{codecWireV2, codecWireV1, false, false},

		{codecJSONV1, codecJSONV1, false, false},
		{codecJSONV2, codecJSONV2, false, false},
		{codecJSONV1, codecJSONV2, false, false},
		{codecJSONV2, codecJSONV1, false, false},

		// Note codecJSONV1WithHack can only be used as a decoder.
		{codecJSONV1, codecJSONV1WithHack, false, false},
		{codecJSONV2, codecJSONV1WithHack, false, false},

		{codecTextV1, codecTextV1, false, false},
		{codecTextV2, codecTextV2, false, false},
		{codecTextV1, codecTextV2, false, false},
		{codecTextV2, codecTextV1, false, false},
	})
}

func TestCodecRoundTripFieldMaskSimple(t *testing.T) {
	t.Parallel()

	msg := &testpb.HelloRequest{
		Name: "Hi",
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"a", "a.b.c"},
		},
	}

	runCodecRoundTripTests(t, msg, []codecTestCase{
		{codecWireV1, codecWireV1, false, false},
		{codecWireV2, codecWireV2, false, false},
		{codecWireV1, codecWireV2, false, false},
		{codecWireV2, codecWireV1, false, false},

		{codecJSONV1, codecJSONV1, false, false},
		{codecJSONV2, codecJSONV2, false, false},
		{codecJSONV1, codecJSONV2, false, true}, // v2 can't decode object field mask
		{codecJSONV2, codecJSONV1, false, true}, // v1 can't decode string field masks

		// Note codecJSONV1WithHack can only be used as a decoder.
		{codecJSONV1, codecJSONV1WithHack, false, false},
		{codecJSONV2, codecJSONV1WithHack, false, false},

		{codecTextV1, codecTextV1, false, false},
		{codecTextV2, codecTextV2, false, false},
		{codecTextV1, codecTextV2, false, false},
		{codecTextV2, codecTextV1, false, false},
	})
}

func TestCodecRoundTripFieldMaskAdvanced(t *testing.T) {
	t.Parallel()

	msg := &testpb.HelloRequest{
		Name: "Hi",
		Fields: &fieldmaskpb.FieldMask{
			Paths: []string{"a.*.b", "a.0.c"},
		},
	}

	runCodecRoundTripTests(t, msg, []codecTestCase{
		{codecWireV1, codecWireV1, false, false},
		{codecWireV2, codecWireV2, false, false},
		{codecWireV1, codecWireV2, false, false},
		{codecWireV2, codecWireV1, false, false},

		{codecJSONV1, codecJSONV1, false, false},
		{codecJSONV2, codecJSONV2, true, false}, // v2 doesn't support advanced field masks
		{codecJSONV1, codecJSONV2, false, true}, // v2 doesn't support advanced field masks
		{codecJSONV2, codecJSONV1, true, false}, // v2 doesn't support advanced field masks

		// Note codecJSONV1WithHack can only be used as a decoder.
		{codecJSONV1, codecJSONV1WithHack, false, false},
		{codecJSONV2, codecJSONV1WithHack, true, false}, // v2 doesn't support advanced field masks

		{codecTextV1, codecTextV1, false, false},
		{codecTextV2, codecTextV2, false, false},
		{codecTextV1, codecTextV2, false, false},
		{codecTextV2, codecTextV1, false, false},
	})
}

func TestFieldMaskJSONDecoder(t *testing.T) {
	t.Parallel()

	simpleStr := `{"fields": "a,b.someStr"}`
	simpleObj := `{"fields": {"paths": ["a", "b.someStr"]}}`
	advancStr := `{"fields": "a,b.*.someStr"}`
	advancObj := `{"fields": {"paths": ["a", "b.*.someStr"]}}`

	simpleFields := []string{"a", "b.some_str"}
	advancFields := []string{"a", "b.*.some_str"}

	cases := []struct {
		body   string
		dec    protoCodec
		fields []string
	}{
		{simpleStr, codecJSONV1, nil},                  // v1 doesn't support string field masks
		{simpleStr, codecJSONV1WithHack, simpleFields}, // v1 with the hack does
		{simpleStr, codecJSONV2, simpleFields},         // v2 does as well

		{simpleObj, codecJSONV1, simpleFields},         // v1 supports object field masks
		{simpleObj, codecJSONV1WithHack, simpleFields}, // v1 supports object field masks
		{simpleObj, codecJSONV2, nil},                  // v2 doesn't support object field masks

		{advancStr, codecJSONV1, nil},                  // v1 doesn't support string field masks
		{advancStr, codecJSONV1WithHack, advancFields}, // v1 with the hack does
		{advancStr, codecJSONV2, nil},                  // v2 doesn't support advanced field masks

		{advancObj, codecJSONV1, advancFields},         // v1 supports advanced object field masks
		{advancObj, codecJSONV1WithHack, advancFields}, // v1 supports advanced object field masks
		{advancObj, codecJSONV2, nil},                  // v2 doesn't support object field masks
	}

	for idx, c := range cases {
		t.Run(fmt.Sprintf("%d %d", idx, c.dec), func(t *testing.T) {
			msg := &testpb.HelloRequest{}
			err := c.dec.Decode([]byte(c.body), msg)
			if c.fields == nil {
				assert.That(t, err != nil, should.BeTrue)
			} else {
				assert.That(t, err, should.ErrLike(nil))
			}
		})
	}
}

type codecTestCase struct {
	enc           protoCodec
	dec           protoCodec
	expectEncFail bool
	expectDecFail bool
}

func runCodecRoundTripTests(t *testing.T, msg *testpb.HelloRequest, cases []codecTestCase) {
	for _, c := range cases {
		for _, initBuf := range [][]byte{nil, []byte("something")} {
			t.Run(fmt.Sprintf("%d-%d", c.enc, c.dec), func(t *testing.T) {
				blob, err := c.enc.Encode(initBuf, msg)
				if c.expectEncFail {
					assert.That(t, err != nil, should.BeTrue)
					return
				}
				assert.That(t, err, should.ErrLike(nil), truth.Explain("%s", string(blob)))
				assert.That(t, bytes.HasPrefix(blob, initBuf), should.BeTrue)

				out := &testpb.HelloRequest{}
				err = c.dec.Decode(blob[len(initBuf):], out)
				if c.expectDecFail {
					assert.That(t, err != nil, should.BeTrue)
					return
				}
				assert.That(t, err, should.ErrLike(nil), truth.Explain("%s", string(blob[len(initBuf):])))
				assert.That(t, out, should.Match(msg))
			})
		}
	}
}
