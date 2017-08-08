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

package isolated

import (
	"encoding/hex"
	"hash"
	"io"
	"os"

	"go.chromium.org/luci/common/api/isolate/isolateservice/v1"
)

// Sum is a shortcut to get a HexDigest from a hash.Hash.
func Sum(h hash.Hash) HexDigest {
	return HexDigest(hex.EncodeToString(h.Sum(nil)))
}

// Hash hashes a reader and returns a HexDigest from it.
func Hash(src io.Reader) (HexDigest, error) {
	h := GetHash()
	_, err := io.Copy(h, src)
	if err != nil {
		return HexDigest(""), err
	}
	return Sum(h), nil
}

// HashBytes hashes content and returns a HexDigest from it.
func HashBytes(content []byte) HexDigest {
	h := GetHash()
	_, _ = h.Write(content)
	return Sum(h)
}

// HashFile hashes a file and returns a HandlersEndpointsV1Digest out of it.
func HashFile(path string) (isolateservice.HandlersEndpointsV1Digest, error) {
	h := GetHash()
	f, err := os.Open(path)
	if err != nil {
		return isolateservice.HandlersEndpointsV1Digest{}, err
	}
	defer f.Close()
	size, err := io.Copy(h, f)
	if err != nil {
		return isolateservice.HandlersEndpointsV1Digest{}, err
	}
	return isolateservice.HandlersEndpointsV1Digest{Digest: string(Sum(h)), IsIsolated: false, Size: size}, nil
}
