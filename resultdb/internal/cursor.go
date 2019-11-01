// Copyright 2019 The LUCI Authors.
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

package internal

import (
	"encoding/base64"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/resultdb/internal/proto"
)

// ParsePageToken extracts a string slice position from the given cursor token.
func ParsePageToken(token string) ([]string, error) {
	if token == "" {
		return nil, nil
	}

	tokBytes, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}

	cursor := &internalpb.Cursor{}
	if err := proto.Unmarshal(tokBytes, cursor); err != nil {
		return nil, err
	}
	return cursor.Position, nil
}

// PageToken converts an string slice representing cursor position to an opaque token string.
func PageToken(pos ...string) string {
	if pos == nil {
		return ""
	}

	msgBytes, err := proto.Marshal(&internalpb.Cursor{Position: pos})
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(msgBytes)
}
