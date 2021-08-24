// Copyright 2021 The LUCI Authors.
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

package pagination

import (
	"encoding/base64"
	"fmt"
	"reflect"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
)

// InvalidToken annotates the error with InvalidArgument appstatus.
func InvalidToken(err error) error {
	return appstatus.Attachf(err, codes.InvalidArgument, "invalid page token")
}

type requestWithPageToken interface {
	GetPageToken() string
}

// ValidatePageToken validates & extracts page token from the request
// into the given proto.
func ValidatePageToken(req requestWithPageToken, dst proto.Message) error {
	return parse(req.GetPageToken(), dst)
}

// schemaVersion is the version of the schema of the token.
//
// Version 1 schema is:
//   Outer envelope: base64.RawURLEncoding.
//   Why: safe to use in URLs as is.
//
//   Inner 1 byte prefix: version.
//   Why: support evolution of the format, e.g. HMAC-ing the cursor.
//
//   Inner remaining: envelope of the serialized binary proto.
//   Why: support arbitrary page tokens.
//
const schemaVersion = 1

// TokenString serializes a generic page token to an opaque URL-safe string.
//
// Input proto can be nil, in which case resulting page token is empty.
func TokenString(src proto.Message) (string, error) {
	if src == nil || reflect.ValueOf(src).IsNil() {
		return "", nil
	}
	// Pre-allocate space to avoid needless re-allocations later.
	// Reserve 1 byte for version set below.
	// 64 bytes total because most tokens should be short.
	bytes := make([]byte, 1, 64)
	bytes[0] = schemaVersion
	bytes, err := proto.MarshalOptions{}.MarshalAppend(bytes, src)
	if err != nil {
		return "", errors.Annotate(err, "failed to serialize page token").Err()
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// parse parses a generic page token into the proto.
//
// Empty page token is valid and results in no changes to the destination proto.
func parse(token string, dst proto.Message) error {
	if token == "" {
		return nil
	}
	bytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return InvalidToken(err)
	}

	switch {
	case len(bytes) == 0:
		return InvalidToken(fmt.Errorf("invalid inner format"))
	case bytes[0] != schemaVersion:
		return InvalidToken(fmt.Errorf("unknown version %d", bytes[0]))
	default:
		bytes = bytes[1:]
	}

	if err = proto.Unmarshal(bytes, dst); err != nil {
		return InvalidToken(err)
	}
	return nil
}
