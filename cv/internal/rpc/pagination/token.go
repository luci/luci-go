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
	"context"
	"reflect"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/secrets"
)

// InvalidToken annotates the error with InvalidArgument appstatus.
func InvalidToken(err error) error {
	return appstatus.Attachf(err, codes.InvalidArgument, "invalid page token")
}

// cryptoAdditionalData is used to verify integrity of the page tokens.
var cryptoAdditionalData = []byte("cv-proto-token")

// DecryptPageToken extracts page token from the request into the given proto.
//
// Returns appstatus-annotated InvalidArgument error if token isn't valid.
func DecryptPageToken(ctx context.Context, pageToken string, dst proto.Message) error {
	if pageToken == "" {
		return nil
	}
	bytes, err := secrets.URLSafeDecrypt(ctx, pageToken, cryptoAdditionalData)
	if err != nil {
		return InvalidToken(err)
	}
	if err := proto.Unmarshal(bytes, dst); err != nil {
		return InvalidToken(err)
	}
	return nil
}

// EncryptPageToken encrypts a generic page token to an opaque URL-safe string,
//
// Input proto can be nil, in which case resulting page token is empty.
func EncryptPageToken(ctx context.Context, src proto.Message) (string, error) {
	if src == nil || reflect.ValueOf(src).IsNil() {
		return "", nil
	}
	bytes, err := proto.Marshal(src)
	if err != nil {
		return "", errors.Fmt("failed to serialize page token: %w", err)
	}
	return secrets.URLSafeEncrypt(ctx, bytes, cryptoAdditionalData)
}
