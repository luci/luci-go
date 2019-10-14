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

package prpc

import (
	"encoding/base64"
	"net/http"
	"strings"
)

// metadataSuffixBinary is a suffix of a gRPC metadata key that specifies that
// the value is decoded using std base64.
// After decoding the value, the suffix must be stripped from the key.
const metadataSuffixBinary = "-bin"

// headerToMeta converts a key-value pair from HTTP format to gRPC metadata.
// Takes care of -metadataSuffixBinary.
func headerToMeta(key, value string) (mdKey, mdValue string, err error) {
	mdKey = strings.ToLower(key)
	if !strings.HasSuffix(mdKey, metadataSuffixBinary) {
		mdValue = value
		return
	}

	mdKey = strings.TrimSuffix(mdKey, metadataSuffixBinary)
	decoded, err := base64.StdEncoding.DecodeString(value)
	mdValue = string(decoded)
	return
}

// metaToHeader converts a key-value pair from gRPC metadata format to HTTP.
// Takes care of -metadataSuffixBinary.
func metaToHeader(key, value string) (hKey, hValue string) {
	if isValidHTTPText(value) {
		return http.CanonicalHeaderKey(key), value
	}

	hKey = http.CanonicalHeaderKey(key + metadataSuffixBinary)
	hValue = base64.StdEncoding.EncodeToString([]byte(value))
	return
}

// isValidHTTPText returns false if s is not valid "TEXT" according to
// https://tools.ietf.org/html/rfc2616#section-2.2
func isValidHTTPText(s string) bool {
	for _, r := range s {
		switch {
		case r == ' ':
			// allowed

		case r < 32, r > 255, r == 127:
			return false
		}
	}

	return true
}
