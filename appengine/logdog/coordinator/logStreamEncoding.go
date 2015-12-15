// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// encodedKeyPrefix is the prefix appended to encoded tag keys. See encodeKey
// for more information.
const encodedKeyPrefix = "Key_"

// encodeKey encodes a key string in a manner that allows it to be used as a
// datastore property.
//
// Datastore properties may consist of [a-zA-Z0-9_]. However, filters on these
// properties require the "=" character to be used, so default base64
// padding is not acceptable. Instead, we will perform the following
// transformation:
// 1) Encode the key with base64's URLEncoding scheme.
// 2) Replace "=" characters with "~".
//
// TODO(dnj): When switching to Go 1.5, use Base64 Encoding w/ "~" as the
//     custom padding character.
func encodeKey(k string) string {
	k = strings.Map(func(r rune) rune {
		if r == '=' {
			return '~'
		}
		return r
	}, base64.URLEncoding.EncodeToString([]byte(k)))
	return strings.Join([]string{encodedKeyPrefix, k}, "")
}

// decodeKey converts an encoded key into its original key string.
func decodeKey(k string) (string, error) {
	if !strings.HasPrefix(k, encodedKeyPrefix) {
		return "", fmt.Errorf("encoded key missing prefix (%s)", encodedKeyPrefix)
	}

	k = strings.Map(func(r rune) rune {
		if r == '~' {
			return '='
		}
		return r
	}, k[len(encodedKeyPrefix):])

	d, err := base64.URLEncoding.DecodeString(k)
	if err != nil {
		return "", fmt.Errorf("failed to decode key: %s", err)
	}
	return string(d), nil
}
