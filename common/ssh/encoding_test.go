// Copyright 2025 The LUCI Authors.
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

package ssh

import (
	"bytes"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testMesasge struct {
	Name    string
	Payload []byte
}

// TestMarshalUnmarshal_RoundTrip tests the "happy path", ensuring that a struct
// can be marshalled and then unmarshalled back to its original state.
func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	t.Parallel()
	msg := testMesasge{
		Name:    "message",
		Payload: []byte{0xDE, 0xCA, 0xCA, 0xFE},
	}

	bytes, err := MarshalBody(msg)
	assert.NoErr(t, err)

	// The expected marshaled data is:
	//   [4-byte] string length, string
	//   [4-byte] []byte length, []byte
	expectedBytes := []byte{
		0, 0, 0, 7, 'm', 'e', 's', 's', 'a', 'g', 'e',
		0, 0, 0, 4, 0xDE, 0xCA, 0xCA, 0xFE,
	}

	assert.That(t, bytes, should.Match(expectedBytes))

	// Unmarshal back into a new struct.
	var unmarshaled testMesasge
	if err := UnmarshalBody(bytes, &unmarshaled); err != nil {
		t.Errorf("UnmarshalBody failed: %v", err)
	}

	assert.That(t, msg.Name, should.Equal(unmarshaled.Name))
	assert.That(t, msg.Payload, should.Match(unmarshaled.Payload))
}

// TestMarshal_UnsupportedTypes checks non-struct values can't be marshaled.
func TestMarshal_UnsupportedTypes(t *testing.T) {
	t.Parallel()
	t.Run("non byte slices in struct", func(t *testing.T) {
		t.Parallel()
		msg := struct {
			Payload []uint32
		}{
			Payload: []uint32{1, 2, 3, 4},
		}

		_, err := MarshalBody(msg)
		assert.ErrIsLike(t, err, "non byte slices aren't supported")
	})

	t.Run("unsupported types in struct", func(t *testing.T) {
		t.Parallel()
		msg := struct {
			Val uint32
		}{
			Val: 1,
		}

		_, err := MarshalBody(msg)
		assert.ErrIsLike(t, err, "unsupported type")
	})

	t.Run("nested struct", func(t *testing.T) {
		t.Parallel()
		msg := struct {
			Nested struct{}
		}{
			Nested: struct{}{},
		}

		_, err := MarshalBody(msg)
		assert.ErrIsLike(t, err, "unsupported type")
	})

	t.Run("non struct", func(t *testing.T) {
		t.Parallel()
		nonStructValues := []any{
			int(1),
			uint32(2),
			"text",
			[]byte{1, 2, 3, 4},
		}

		for _, v := range nonStructValues {
			_, err := MarshalBody(v)
			assert.ErrIsLike(t, err, "lengthprefix encoding only supports structs")
		}
	})
}

// TestUnmarshalBody_Errors tests error conditions for the unmarshalling
// function.
func TestUnmarshalBody_Errors(t *testing.T) {
	bytes, err := MarshalBody(testMesasge{
		Name:    "test",
		Payload: []byte{},
	})
	if err != nil {
		t.Fatalf("failed to marshal test data: %v", err)
	}

	t.Parallel()

	testCases := []struct {
		name        string
		inputData   []byte
		target      interface{}
		expectedErr string
	}{
		{
			name:        "Unmarshal to non-pointer",
			inputData:   bytes,
			target:      testMesasge{},
			expectedErr: "expects a pointer to a struct",
		},
		{
			name:        "Unmarshal to pointer to non-struct",
			inputData:   bytes,
			target:      new(string),
			expectedErr: "expects a pointer to a struct",
		},
		{
			name: "Unmarshal data with invalid length prefix (too short)",
			// Data contains only 2 bytes, not the 4 needed for length
			inputData:   []byte{0, 0},
			target:      &testMesasge{},
			expectedErr: "failed to read byte slice length for field Name: unexpected EOF",
		},
		{
			name: "Unmarshal data with mismatched length (data too short)",
			// Declares length of 10, but only provides 5 bytes
			inputData:   []byte{0, 0, 0, 10, 'd', 'a', 't', 'a'},
			target:      &testMesasge{},
			expectedErr: "failed to read byte slice length for field Name: unexpected EOF",
		},
		{
			name: "Unmarshal to struct with unsupported field type",
			// Marshal a valid struct, but unmarshal into a struct with an int
			inputData:   bytes,
			target:      &struct{ Age int }{},
			expectedErr: "unsupported type for field Age: int",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := UnmarshalBody(tc.inputData, tc.target)
			assert.ErrIsLike(t, err, tc.expectedErr)
		})
	}
}

// TestDecode_DataLengthLimit tests the data exceeding maxDataLength limit
// triggers an error.
func TestDecode_DataLengthLimit(t *testing.T) {
	t.Parallel()

	t.Run("data length limit", func(t *testing.T) {
		t.Parallel()

		data := bytes.NewBuffer([]byte{0xff, 0x00, 0x00, 0x00, 0x12, 0x34, 0x56, 0x78})
		_, err := decodeLengthPrefix(data)

		assert.ErrIsLike(t, err, "data length is larger than the defined limit")
	})
}
