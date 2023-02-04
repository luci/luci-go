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

package lex64

import (
	"bytes"
	"encoding/base64"
	"io"

	"go.chromium.org/luci/common/errors"
)

// doEncode is a utility method that takes an encoding and a message and encodes
// it if possible.
func doEncode(encoding *base64.Encoding, input []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	encoder := base64.NewEncoder(encoding, buf)
	_, err := io.Copy(encoder, bytes.NewReader(input))
	if err := errors.Append(err, encoder.Close()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// doDecode is a utility method that takes an encoded message and decodes it if possible.
func doDecode(encoding *base64.Encoding, encoded []byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	decoder := base64.NewDecoder(encoding, bytes.NewReader(encoded))
	if _, err := io.Copy(buf, decoder); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
