// Copyright 2017 The LUCI Authors.
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

package buildstore

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
)

// encode marshals src to JSON and compresses it.
func encode(src interface{}) ([]byte, error) {
	jsoninsh, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	gsw := gzip.NewWriter(&buf)
	_, err = gsw.Write(jsoninsh)
	if err != nil {
		return nil, err
	}
	err = gsw.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decode decompresses data and unmarshals into dest as JSON.
func decode(dest interface{}, data []byte) error {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	err = json.NewDecoder(reader).Decode(dest)
	if err != nil {
		return err
	}
	return reader.Close()
}
