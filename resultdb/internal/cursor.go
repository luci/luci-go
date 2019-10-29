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
	"bytes"
	"crypto/sha256"
	"encoding/base64"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/proto"
)

const curVersion = 0

// NewCursor creates a cursor in position 0 for the specified set of pagination
// requests and given page size.
func NewCursor(reqBytes []byte, pageSize int32) *internalpb.Cursor {
	if pageSize <= 0 {
		panic("nonpositive cursor page size")
	}

	h := sha256.New()
	h.Write(reqBytes)
	return &internalpb.Cursor{
		RequestHash: h.Sum(nil),
		PageSize:    pageSize,
	}
}

// Cursor creates a cursor from the given cursor token.
func Cursor(token string) (*internalpb.Cursor, error) {
	tokBytes, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, err
	}
	buf := proto.NewBuffer(tokBytes)

	switch version, err := buf.DecodeVarint(); {
	case err != nil:
		return nil, errors.Annotate(err, "cursor version").Err()
	case version != 0:
		return nil, errors.Reason("unknown cursor version %d", version).Err()
	}

	cursor := &internalpb.Cursor{}
	if err := buf.DecodeMessage(cursor); err != nil {
		return nil, errors.Annotate(err, "cursor contents %q", buf.Bytes()).Err()
	}
	return cursor, nil
}

// Token converts an internally represented cursor to an opaque token string.
func Token(c *internalpb.Cursor) (string, error) {
	buf := proto.NewBuffer(nil)
	buf.EncodeVarint(curVersion)
	if err := buf.EncodeMessage(c); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// ValidateCursor indicates whether this cursor may be used with the request
// represented by the given bytes.
func ValidateCursor(c *internalpb.Cursor, req []byte) bool {
	h := sha256.New()
	h.Write(req)
	return bytes.Compare(c.RequestHash, h.Sum(nil)) == 0
}
