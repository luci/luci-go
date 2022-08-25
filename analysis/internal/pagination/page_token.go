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

package pagination

import (
	"encoding/base64"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/grpc/appstatus"

	paginationpb "go.chromium.org/luci/analysis/internal/pagination/proto"
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

	msg := &paginationpb.PageToken{}
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

	msgBytes, err := proto.Marshal(&paginationpb.PageToken{Position: pos})
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(msgBytes)
}

// InvalidToken annotates the error with InvalidArgument appstatus.
func InvalidToken(err error) error {
	return appstatus.Attachf(err, codes.InvalidArgument, "invalid page_token")
}
