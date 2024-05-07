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

package pagination

import (
	"encoding/base64"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	internalpb "go.chromium.org/luci/resultdb/internal/proto"
)

const (
	// pageSizeMax is the maximum pageSize.
	// It is high, but still required to prevent client-caused OOMs.
	pageSizeMax     = 10_000
	pageSizeDefault = 1000
)

// ParseToken extracts a string slice position from the given page token.
// May return an appstatus-annotated error.
func ParseToken(token string) ([]string, error) {
	if token == "" {
		return nil, nil
	}

	tokBytes, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, InvalidToken(err)
	}

	msg := &internalpb.PageToken{}
	if err := proto.Unmarshal(tokBytes, msg); err != nil {
		return nil, InvalidToken(err)
	}
	return msg.Position, nil
}

// Token converts an string slice representing page token position to an opaque
// token string.
func Token(pos ...string) string {
	if pos == nil {
		return ""
	}

	msgBytes, err := proto.Marshal(&internalpb.PageToken{Position: pos})
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(msgBytes)
}

// AdjustPageSize takes the given requested pageSize and adjusts as necessary.
func AdjustPageSize(pageSize int32) int {
	switch {
	case pageSize >= pageSizeMax:
		return pageSizeMax
	case pageSize > 0:
		return int(pageSize)
	default:
		return pageSizeDefault
	}
}

// ValidatePageSize returns a non-nil error if pageSize is invalid.
// Returns nil if pageSize is 0.
func ValidatePageSize(pageSize int32) error {
	if pageSize < 0 {
		return errors.Reason("negative").Err()
	}
	return nil
}

// InvalidToken annotates the error with InvalidArgument appstatus.
func InvalidToken(err error) error {
	return appstatus.Attachf(err, codes.InvalidArgument, "invalid page_token")
}
