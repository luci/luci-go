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

package internal

import (
	"context"
	"crypto/rand"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/loginsessions/internal/statepb"
	"go.chromium.org/luci/server/secrets"
)

const aeadContextState = "loginsessions:state"

// RandomBlob generates a completely random byte string of given length.
func RandomBlob(bytes int) []byte {
	blob := make([]byte, bytes)
	if _, err := rand.Read(blob); err != nil {
		panic(fmt.Sprintf("failed to generate random string: %s", err))
	}
	return blob
}

// RandomAlphaNum generates a random alphanumeric string of given length.
//
// Its entropy is ~6*size random bits.
func RandomAlphaNum(size int) string {
	// Note: there are exactly 64 symbols here with two extra '0' to stay in the
	// alphanumeric range.
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ012345678900"
	blob := RandomBlob(size)
	for i := range blob {
		blob[i] = alphabet[blob[i]&63]
	}
	return string(blob)
}

// EncryptState serializes, encrypts and base64-encodes OpenIDState.
func EncryptState(ctx context.Context, msg *statepb.OpenIDState) (string, error) {
	plain, err := proto.Marshal(msg)
	if err != nil {
		return "", err
	}
	return secrets.URLSafeEncrypt(ctx, plain, []byte(aeadContextState))
}

// DecryptState is the reverse of EncryptState.
func DecryptState(ctx context.Context, enc string) (*statepb.OpenIDState, error) {
	plain, err := secrets.URLSafeDecrypt(ctx, enc, []byte(aeadContextState))
	if err != nil {
		return nil, err
	}
	var msg statepb.OpenIDState
	if err := proto.Unmarshal(plain, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
