// Copyright 2018 The LUCI Authors.
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

package metadata

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"

	"google.golang.org/protobuf/proto"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

// CalculateFingerprint fills in Fingerprint field of the given metadata.
//
// It is basically base64-encoded SHA1 digest of the serialized proto
// (excluding 'fingerprint' field itself). It doesn't have to be
// cryptographically secure.
//
// Note that it is also fine if the proto encoding changes with time. We don't
// care about exact meaning of the fingerprint, as long as it changes each time
// we update the metadata (so strictly speaking we can just generate random
// strings and call them fingerprints).
func CalculateFingerprint(m *repopb.PrefixMetadata) {
	m.Fingerprint = ""
	blob, err := proto.Marshal(m)
	if err != nil {
		panic(fmt.Sprintf("failed to proto-marshal PrefixMetadata for prefix %q - %s", m.Prefix, err))
	}
	h := sha1.New()
	h.Write([]byte("PrefixMetadata:"))
	h.Write(blob)
	m.Fingerprint = base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
