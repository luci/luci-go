// Copyright 2015 The LUCI Authors.
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

package coordinator

import (
	"encoding/base64"
	"fmt"
	"strings"
)

// encodedKeyPrefix is the prefix appended to encoded tag keys. See encodeKey
// for more information.
const encodedKeyPrefix = "Key_"

var keyEncoding = base64.URLEncoding.WithPadding('~')

// encodeKey encodes a key string in a manner that allows it to be used as a
// datastore property.
//
// Datastore properties may consist of [a-zA-Z0-9_]. However, filters on these
// properties require the "=" character to be used, so default base64
// padding is not acceptable.
func encodeKey(k string) string {
	return (encodedKeyPrefix + keyEncoding.EncodeToString([]byte(k)))
}

// decodeKey converts an encoded key into its original key string.
func decodeKey(k string) (string, error) {
	if !strings.HasPrefix(k, encodedKeyPrefix) {
		return "", fmt.Errorf("encoded key missing prefix (%s)", encodedKeyPrefix)
	}

	data, err := keyEncoding.DecodeString(k[len(encodedKeyPrefix):])
	if err != nil {
		return "", fmt.Errorf("failed to decode key: %v", err)
	}
	return string(data), nil
}
