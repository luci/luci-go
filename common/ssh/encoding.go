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
	"encoding/binary"
	"fmt"
	"io"
	"reflect"
)

// maxDataLength sets the maximum length that the decoder is willing to decode
// to avoid silent out-of-memory errors.
//
// We don't expect a single SSH message to exceed this size.
const maxDataLength = uint32(32 * 1024 * 1024)

var messageByteOrder = binary.BigEndian

// encodeLengthPrefix writes a 4-byte length prefix of data `b`, then `b`
// itself.
func encodeLengthPrefix(w io.Writer, b []byte) error {
	if err := binary.Write(w, messageByteOrder, uint32(len(b))); err != nil {
		return err
	}

	_, err := w.Write(b)
	return err
}

// decodeLengthPrefix reads a 4-byte length prefix and the `length` bytes of
// data that follows.
func decodeLengthPrefix(r io.Reader) ([]byte, error) {
	var length uint32
	if err := binary.Read(r, messageByteOrder, &length); err != nil {
		return nil, err
	}

	if length > maxDataLength {
		return nil, fmt.Errorf("data length is larger than the defined limit, got length: %v, max: %v", length, maxDataLength)
	}

	b := make([]byte, length)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}

	return b, nil
}

// MarshalBody encodes struct `v` according to SSH message encoding.
//
// It essentially adds a 4-byte length prefix to variable length fields like
// string or []byte.
func MarshalBody(v any) ([]byte, error) {
	val := reflect.ValueOf(v)

	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return nil, fmt.Errorf("lengthprefix encoding only supports structs, got %s", val.Kind())
	}

	var buf bytes.Buffer

	for i := range val.NumField() {
		fieldVal := val.Field(i)
		switch fieldVal.Kind() {
		case reflect.String:
			if err := encodeLengthPrefix(&buf, []byte(fieldVal.String())); err != nil {
				return nil, err
			}

		case reflect.Slice:
			// Only byte slice is supported.
			if fieldVal.Type().Elem().Kind() != reflect.Uint8 {
				return nil, fmt.Errorf("non byte slices aren't supported, got: %v", fieldVal.Type())
			}

			if err := encodeLengthPrefix(&buf, fieldVal.Bytes()); err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf("unsupported type: %s", fieldVal.Kind())
		}
	}

	return buf.Bytes(), nil
}

// UnmarshalBody decodes SSH message payload into a struct.
//
// The 'v' argument must be a pointer to a struct.
func UnmarshalBody(data []byte, v any) error {
	ptrVal := reflect.ValueOf(v)
	if ptrVal.Kind() != reflect.Ptr {
		return fmt.Errorf("expects a pointer to a struct, got %T", v)
	}
	val := ptrVal.Elem()
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("expects a pointer to a struct, got pointer to %s", val.Kind())
	}

	r := bytes.NewReader(data)
	t := val.Type()

	for i := range val.NumField() {
		fieldVal := val.Field(i)
		fieldType := t.Field(i)

		// Skip unexported fields or fields that cannot be set
		if !fieldType.IsExported() || !fieldVal.CanSet() {
			continue
		}

		switch fieldVal.Kind() {
		case reflect.String:
			b, err := decodeLengthPrefix(r)
			if err != nil {
				return fmt.Errorf("failed to read byte slice length for field %s: %w", fieldType.Name, err)
			}
			fieldVal.SetString(string(b))

		case reflect.Slice:
			if fieldVal.Type().Elem().Kind() != reflect.Uint8 {
				return fmt.Errorf("only byte slices are supported, got: %v", fieldVal.Type())
			}

			b, err := decodeLengthPrefix(r)
			if err != nil {
				return fmt.Errorf("failed to read byte slice length for field %s: %w", fieldType.Name, err)
			}
			fieldVal.SetBytes(b)

		default:
			return fmt.Errorf("unsupported type for field %s: %s", fieldType.Name, fieldVal.Kind())
		}
	}

	return nil
}
