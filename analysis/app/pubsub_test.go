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

package app

import (
	"bytes"
	"encoding/json"
	"io"

	// Needed to ensure task class is registered.
	_ "go.chromium.org/luci/analysis/internal/services/verdictingester"
)

func makeReq(blob []byte, attributes map[string]any) io.ReadCloser {
	msg := struct {
		Message struct {
			Data       []byte
			Attributes map[string]any
		}
	}{struct {
		Data       []byte
		Attributes map[string]any
	}{Data: blob, Attributes: attributes}}
	jmsg, _ := json.Marshal(msg)
	return io.NopCloser(bytes.NewReader(jmsg))
}
