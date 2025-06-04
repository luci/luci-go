// Copyright 2025 The LUCI Authors.
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

package proxyserver

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/keyset"
	"github.com/google/tink/go/tink"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
)

// CASURLObfuscator takes real GCS signed URLs and transforms them into URLs
// pointing to the proxy (and vice-versa).
//
// The intention is twofold:
//  1. Make sure the proxy client can't just abuse the proxy to fetch arbitrary
//     GCS files (without keeping track of every single file in the proxy
//     state).
//  2. Hide raw GCS signed URLs from the proxy client it case it is undesirable
//     for the proxy client to have direct GCS URL.
type CASURLObfuscator struct {
	aead tink.AEAD
}

// obfuscatorAEADCtx is used as additional AEAD data.
const obfuscatorAEADCtx = "cipd:proxy:ProxiedCASObject"

// NewCASURLObfuscator constructs an obfuscator with a new random AEAD key.
func NewCASURLObfuscator() *CASURLObfuscator {
	kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		panic(err)
	}
	aead, err := aead.New(kh)
	if err != nil {
		panic(err)
	}
	return &CASURLObfuscator{aead: aead}
}

// Obfuscate returns "http://<ProxiedCASDomain>/obj/<encrypted obj>".
func (ob *CASURLObfuscator) Obfuscate(obj *proxypb.ProxiedCASObject) (string, error) {
	blob, err := proto.Marshal(obj)
	if err != nil {
		return "", err
	}
	cipher, err := ob.aead.Encrypt(blob, []byte(obfuscatorAEADCtx))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("http://%s/obj/%s", ProxiedCASDomain, base64.RawURLEncoding.EncodeToString(cipher)), nil
}

// Unobfuscate reverses what Obfuscate did.
func (ob *CASURLObfuscator) Unobfuscate(url string) (*proxypb.ProxiedCASObject, error) {
	b64, found := strings.CutPrefix(url, fmt.Sprintf("http://%s/obj/", ProxiedCASDomain))
	if !found {
		return nil, errors.New("unrecognized URL")
	}
	cipher, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return nil, errors.Fmt("bad base64: %w", err)
	}
	blob, err := ob.aead.Decrypt(cipher, []byte(obfuscatorAEADCtx))
	if err != nil {
		return nil, errors.Fmt("unrecognized CAS object reference (was the CIPD proxy restarted?): %w", err)
	}
	var obj proxypb.ProxiedCASObject
	if err = proto.Unmarshal(blob, &obj); err != nil {
		// Should really be impossible.
		return nil, errors.Fmt("bad ProxiedCASObject: %w", err)
	}
	return &obj, nil
}
