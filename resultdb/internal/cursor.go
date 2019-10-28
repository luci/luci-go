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

const curVersion = 0

// Token converts an internally represented cursor to an opaque token string.
func Token(cursor *internal.Cursor) (string, error) {
	tokBytes, err := proto.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(tokBytes), nil
}

// Offset extracts the offset from a Cursor token.
func Offset(cursorTok string) (int64, error) {
	tokBytes, err := base64.StdEncoding.DecodeString(cursorTok)
	if err != nil {
		return 0, err
	}

	cursor := &internal.Cursor{}
	if err := proto.Unmarshal(tokBytes, cursor); err != nil {
		return 0, err
	}

	if cursor.Version != 0 {
		return 0, errors.Reason("unknown cursor version %d", cursor.Version).Err()
	}

	return cursor.Offset, nil
}
