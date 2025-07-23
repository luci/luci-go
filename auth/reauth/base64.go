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

package reauth

import (
	"encoding/base64"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/webauthn"
)

// base64Encoding is an interface for base64 encodings.
type base64Encoding interface {
	DecodeString(string) ([]byte, error)
	EncodeToString([]byte) string
}

// transcodeBase64 transcodes a string from one base64 encoding to another.
func transcodeBase64(to, from base64Encoding, s string) (string, error) {
	d, err := from.DecodeString(s)
	if err != nil {
		return "", err
	}
	return to.EncodeToString(d), nil
}

// base64 encoding used by ReAuth.
var reauthEncoding = base64.StdEncoding

// PluginFromReAuthBase64 converts a ReAuth base64 string to a plugin base64 string.
func PluginFromReAuthBase64(s string) (string, error) {
	s2, err := transcodeBase64(webauthn.Base64Encoding, reauthEncoding, s)
	if err != nil {
		return "", errors.Fmt("PluginFromReAuthBase64: %w", err)
	}
	return s2, nil
}

// ReAuthFromPluginBase64 converts a plugin base64 string to a ReAuth base64 string.
func ReAuthFromPluginBase64(s string) (string, error) {
	s2, err := transcodeBase64(reauthEncoding, webauthn.Base64Encoding, s)
	if err != nil {
		return "", errors.Fmt("ReAuthFromPluginBase64: %w", err)
	}
	return s2, nil
}
