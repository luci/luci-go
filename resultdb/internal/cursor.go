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

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/proto"
)

// NewCursor creates a cursor starting at the given Key (inclusive).
func NewCursor(pos []string) *internalpb.Cursor {
	return &internalpb.Cursor{Position: pos}
}

// Cursor creates a cursor from the given cursor token.
func Cursor(token string) (*internalpb.Cursor, error) {
	if token == "" {
		return NewCursor(nil), nil
	}

	tokBytes, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}

	buf := proto.NewBuffer(tokBytes)
	cursor := &internalpb.Cursor{}
	if err := buf.DecodeMessage(cursor); err != nil {
		return nil, errors.Annotate(err, "cursor contents %q", buf.Bytes()).Err()
	}
	return cursor, nil
}

// Token converts an internally represented cursor to an opaque token string.
func Token(c *internalpb.Cursor) (string, error) {
	if c.GetPosition() == nil {
		return "", nil
	}

	buf := proto.NewBuffer(nil)
	if err := buf.EncodeMessage(c); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
