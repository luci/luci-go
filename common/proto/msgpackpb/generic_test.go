// Copyright 2022 The LUCI Authors.
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

package msgpackpb

import (
	"bytes"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRoundtrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		input   *TestMessage
		err     string
		raw     []byte
		options []Option
	}{
		{
			name: "scalar fields",
			input: &TestMessage{
				Boolval:       true,
				Intval:        -100,
				Uintval:       100,
				ShortIntval:   -50,
				ShortUintval:  50,
				Floatval:      6.28318531,
				ShortFloatval: 3.1415,
				Strval:        "hi",
				Value:         VALUE_ONE,
			},
			raw: []byte{
				137,    // 9 element map
				2, 195, // tag 2, true
				3, 208, 156, // tag 3, -100
				4, 100, // tag 4, 100
				5, 208, 206, // tag 5, -50
				6, 50, // tag 6, 50
				7, 162, 104, 105, // tag 7, "hi"
				8, 203, 64, 25, 33, 251, 84, 116, 161, 104, // tag 8, 6.28318531
				9, 202, 64, 73, 14, 86, // tag 9, 3.1415
				10, 1}, // tag 10, 1
			options: []Option{Deterministic},
		},

		{
			name: "repeated simple",
			input: &TestMessage{
				Strings: []string{"hello", "there"},
			},
			raw: []byte{
				129,     // 1 element map
				13, 146, // tag 13, 2 element array
				165, 104, 101, 108, 108, 111, // "hello"
				165, 116, 104, 101, 114, 101, // "there"
			},
			options: []Option{Deterministic},
		},

		{
			name: "embedded message",
			input: &TestMessage{
				SingleRecurse: &TestMessage{
					SingleRecurse: &TestMessage{
						Strval: "hello",
					},
				},
			},
			raw: []byte{
				129,                             // 1 element map
				14,                              // tag 13
				129,                             // 1 element map
				14,                              // tag 13
				129,                             // 1 element map
				7, 165, 104, 101, 108, 108, 111, // tag 7, "hello"
			},
			options: []Option{Deterministic},
		},

		{
			name: "external message",
			input: &TestMessage{
				Duration: &durationpb.Duration{
					Seconds: 10000,
					Nanos:   10000,
				},
			},
			raw: []byte{
				129,     // 1 element map
				12, 146, // tag 12, 2 element ARRAY, since this message is encoded like a lua 'array'
				205, 39, 16, // (implicit tag 1), 10000
				205, 39, 16, // (implicit tag 2), 10000
			},
			options: []Option{Deterministic},
		},

		{
			name: "map",
			input: &TestMessage{
				Mapfield: map[string]*TestMessage{
					"hello":   {Strval: "there"},
					"general": {Strval: "kinobi..."},
				},
			},
			raw: []byte{
				129,     // 1 element map
				11, 130, // tag 11, 2 entry map
				167, 103, 101, 110, 101, 114, 97, 108, // "general"
				129,                                             // 2 element map
				7, 169, 107, 105, 110, 111, 98, 105, 46, 46, 46, // tag 7, "kenobi..."
				165, 104, 101, 108, 108, 111, // "hello"
				129,                             // 1 element map
				7, 165, 116, 104, 101, 114, 101, // tag 7, "there"
			},
			options: []Option{Deterministic},
		},

		{
			name: "intern",
			input: &TestMessage{
				Strval: "am interned",
				Mapfield: map[string]*TestMessage{
					"another": {Boolval: true},
					"not":     {Boolval: false},
				},
				SingleRecurse: &TestMessage{
					Strval: "also not",
				},
			},
			raw: []byte{
				131,  // 3 element map
				7, 0, // tag 7, interned string 0
				11, 130, // tag 11, 2 element map
				1, 129, 2, 195, // interned string 1, 1 element map, tag 2, true
				163, 110, 111, 116, 128, // "not", zero element map.
				14, 129, // tag 14, 1 element map
				7, 168, 97, 108, 115, 111, 32, 110, 111, 116, // tag 7, "also not"
			},
			options: []Option{Deterministic, WithStringInternTable([]string{
				"am interned",
				"another",
			})},
		},
	}

	ftt.Run(`TestRoundtrip`, t, func(t *ftt.Test) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *ftt.Test) {
				raw, err := Marshal(tc.input, tc.options...)
				if tc.err == "" {
					assert.Loosely(t, err, should.BeNil)
				} else {
					assert.Loosely(t, err, should.ErrLike(tc.err))
					return
				}

				if tc.raw != nil {
					assert.Loosely(t, []byte(raw), should.Match(tc.raw))
				}

				msg := &TestMessage{}
				assert.Loosely(t, Unmarshal(raw, msg, tc.options...), should.BeNil)

				assert.Loosely(t, msg, should.Match(tc.input))
			})
		}
	})

}

func TestEncode(t *testing.T) {
	t.Parallel()

	ftt.Run(`TestEncode`, t, func(t *ftt.Test) {
		t.Run(`unknown fields`, func(t *ftt.Test) {
			// use Duration which encodes seconds with field 1, which is reserved.
			enc, err := proto.Marshal(durationpb.New(20 * time.Second))
			assert.Loosely(t, err, should.BeNil)

			tm := &TestMessage{}
			assert.Loosely(t, proto.Unmarshal(enc, tm), should.BeNil)

			assert.Loosely(t, tm.ProtoReflect().GetUnknown(), should.NotBeEmpty)

			_, err = Marshal(tm)
			assert.Loosely(t, err, should.ErrLike("unknown non-msgpack fields"))
		})
	})
}

// TestDecode tests the pathway from msgpack -> proto, focusing on pathways
// where the msgpack message contains a different encoded value than the target
// field.
func TestDecode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		tweakEnc      func(*msgpack.Encoder)
		input         any // will be encoded verbatim with
		expect        *TestMessage
		expectUnknown protoreflect.RawFields
		expectRaw     msgpack.RawMessage
		expectDecoded any
		err           string
	}{
		{
			name: "int32->int64",
			input: map[int32]any{
				3: int32(10),
			},
			expect: &TestMessage{Intval: 10},
		},
		{
			name: "int8->int64",
			input: map[int32]any{
				3: int8(10),
			},
			expect: &TestMessage{Intval: 10},
		},
		{
			name: "int64->int32",
			input: map[int32]any{
				5: int64(10),
			},
			expect: &TestMessage{ShortIntval: 10},
		},
		{
			name: "int64->int32 (overflow)",
			input: map[int32]any{
				5: int64(math.MaxInt32 * 2),
			},
			expect: &TestMessage{ShortIntval: -2},
		},
		{
			name: "float64->int32",
			input: map[int32]any{
				5: float64(217),
			},
			err: "bad type: expected int32, got float64",
		},

		{
			name: "unknown field",
			input: map[int32]any{
				777: "nerds",
				3:   100,
			},
			expect: &TestMessage{
				Intval: 100,
			},
			expectUnknown: []byte{
				250, 255, 255, 255, 15, // proto: 536870911: LEN
				10,        // proto: 10 bytes in this field
				129,       // msgpack: 1 element map
				205, 3, 9, // msgpack: 777
				165, 110, 101, 114, 100, 115, // msgpack: 5-char string, "nerds"
			},
			expectRaw: []byte{
				130,    // 2 item map
				3, 100, // tag 3, 100
				205, 3, 9, 165, 110, 101, 114, 100, 115, // tag 777, 5 char string "nerds"
			},
			expectDecoded: map[int32]any{
				3:   int64(100),
				777: "nerds",
			},
		},

		{
			name: "sparse array",
			input: map[int32]any{
				13: map[int32]string{
					3:  "hello",
					12: "there",
				},
			},
			expect: &TestMessage{
				Strings: []string{
					"", "", "",
					"hello",
					"", "", "",
					"", "", "",
					"", "",
					"there",
				},
			},
		},
	}

	ftt.Run(`TestDecode`, t, func(t *ftt.Test) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *ftt.Test) {
				enc := msgpack.GetEncoder()
				defer msgpack.PutEncoder(enc)

				buf := bytes.Buffer{}
				enc.Reset(&buf)
				if tc.tweakEnc != nil {
					tc.tweakEnc(enc)
				}
				assert.Loosely(t, enc.Encode(tc.input), should.BeNil)

				msg := &TestMessage{}
				err := Unmarshal(buf.Bytes(), msg)
				if tc.err == "" {
					assert.Loosely(t, err, should.BeNil)

					known := proto.Clone(msg).(*TestMessage)
					known.ProtoReflect().SetUnknown(nil)
					assert.Loosely(t, known, should.Match(tc.expect))

					assert.Loosely(t, msg.ProtoReflect().GetUnknown(), should.Match(tc.expectUnknown))

					if tc.expectRaw != nil {
						raw, err := Marshal(msg, Deterministic)
						assert.Loosely(t, err, should.BeNil)

						assert.Loosely(t, raw, should.Match(tc.expectRaw))

						if len(msg.ProtoReflect().GetUnknown()) > 0 {
							dec := msgpack.GetDecoder()
							defer msgpack.PutDecoder(dec)
							dec.Reset(bytes.NewBuffer(raw))
							dec.UseLooseInterfaceDecoding(true)
							dec.SetMapDecoder(func(d *msgpack.Decoder) (any, error) {
								return d.DecodeUntypedMap()
							})

							decoded := reflect.MakeMap(reflect.TypeOf(tc.expectDecoded))

							assert.Loosely(t, dec.DecodeValue(decoded), should.BeNil)

							assert.Loosely(t, decoded.Interface(), should.Match(tc.expectDecoded))
						}
					}
				} else {
					assert.Loosely(t, err, should.ErrLike(tc.err))
				}

			})
		}
	})

}
