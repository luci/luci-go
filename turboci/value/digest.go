// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package value

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/types/known/anypb"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

var (
	anyTypeURLTag = protowire.AppendTag(nil, 1, protowire.BytesType)
	anyValueTag   = protowire.AppendTag(nil, 2, protowire.BytesType)
)

// Digest is a typed form of a serialized TurboCI ValueDigest.
type Digest string

// ToProto decodes this Digest to its protobuf form.
func (d Digest) ToProto() (*orchestratorpb.ValueDigest, error) {
	buf, err := base64.RawURLEncoding.DecodeString(string(d))
	if err != nil {
		return nil, err
	}

	// peel off the algo (which must be VALUE_HASH_ALGO_SHA256)
	if len(buf) == 0 {
		return nil, fmt.Errorf("missing algorithm")
	}
	if algo := buf[len(buf)-1]; algo != byte(orchestratorpb.ValueHashAlgo_VALUE_HASH_ALGO_SHA256) {
		return nil, fmt.Errorf("bad algorithm: 0x%x", algo)
	}
	buf = buf[:len(buf)-1]

	// peel off the raw hash from the front
	if len(buf) < sha256.Size {
		return nil, fmt.Errorf("insufficient bytes for hash")
	}
	raw := buf[:sha256.Size]

	buf = buf[sha256.Size:]
	siz, num := binary.Varint(buf)
	if num != len(buf) {
		return nil, fmt.Errorf("extra bytes while decoding size")
	}

	return orchestratorpb.ValueDigest_builder{
		Hash:      raw,
		SizeBytes: &siz,
		Algo:      orchestratorpb.ValueHashAlgo_VALUE_HASH_ALGO_SHA256.Enum(),
	}.Build(), nil
}

// ComputeDigest computes and returns the Digest for a given `Any` proto message.
//
// This does not check the integrity of `apb` in any way.
func ComputeDigest(apb *anypb.Any) Digest {
	typeURL := apb.GetTypeUrl()
	value := apb.GetValue()

	// hash the `apb` directly to avoid extra allocations:
	//   protoTag(1, bytes)
	//   type_url
	//   protoTag(2, bytes)
	//   value
	hsh := sha256.New()
	hsh.Write(anyTypeURLTag)
	hsh.Write([]byte(typeURL))
	hsh.Write(anyValueTag)
	hsh.Write(value)

	buf := make([]byte, 0, hsh.Size()+binary.MaxVarintLen64+1)
	buf = hsh.Sum(buf)
	buf = binary.AppendVarint(buf, int64(len(anyTypeURLTag)+len(anyValueTag)+len(typeURL)+len(value)))
	buf = append(buf, byte(orchestratorpb.ValueHashAlgo_VALUE_HASH_ALGO_SHA256))
	return Digest(base64.RawURLEncoding.EncodeToString(buf))
}
