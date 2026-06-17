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
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"

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
	siz, num := binary.Uvarint(buf)
	if num != len(buf) {
		return nil, fmt.Errorf("extra bytes while decoding size")
	}

	return orchestratorpb.ValueDigest_builder{
		Hash:      raw,
		SizeBytes: &siz,
		Algo:      orchestratorpb.ValueHashAlgo_VALUE_HASH_ALGO_SHA256.Enum(),
	}.Build(), nil
}

func writeAnyDeterministically[W io.Writer](apb *anypb.Any, w W) int {
	// write the `apb` fields to avoid extra allocations and avoid proto
	// reflection, since Any is so simple:
	//
	//   protoTag(1, bytes)
	//   uvarint(len(type_url))
	//   type_url
	//   protoTag(2, bytes)
	//   uvarint(len(value))
	//   value
	//
	// NOTE: We omit empty fields per the behavior of proto2/proto3 encoding.
	// This is important to accurately calculate hash and size, because otherwise
	// we would end up encoding an extra [TAG 0] pair of bytes for empty-values.

	size := 0
	write := func(b []byte) {
		_, err := w.Write(b)
		if err != nil {
			// NOTE: We can ignore errors from w.Write because this is only used with
			// sha256's hash.Hash and bytes.Buffer, neither of which can actually return
			// an error.
			panic(err)
		}
		size += len(b)
	}

	// In theory, the compiler should be smart enough to allocate this on the
	// stack and prove that it does not escape.
	varintBuf := make([]byte, 0, binary.MaxVarintLen64)

	if typeURL := apb.GetTypeUrl(); len(typeURL) > 0 {
		write(anyTypeURLTag)
		write(protowire.AppendVarint(varintBuf, uint64(len(typeURL))))
		write([]byte(typeURL))
	}

	if value := apb.GetValue(); len(value) > 0 {
		write(anyValueTag)
		write(protowire.AppendVarint(varintBuf, uint64(len(value))))
		write(value)
	}

	return size
}

// ComputeDigest computes and returns the Digest for a given `Any` proto message.
//
// This does not check the integrity of `apb` in any way.
func ComputeDigest(apb *anypb.Any) Digest {
	hsh := sha256.New()
	size := writeAnyDeterministically(apb, hsh)

	buf := make([]byte, 0, hsh.Size()+binary.MaxVarintLen64+1)
	buf = hsh.Sum(buf)
	buf = binary.AppendUvarint(buf, uint64(size))
	buf = append(buf, byte(orchestratorpb.ValueHashAlgo_VALUE_HASH_ALGO_SHA256))
	return Digest(base64.RawURLEncoding.EncodeToString(buf))
}

// DeterministicallySerializeAny is like `proto.Marshal` for `Any`, except:
//   - it cannot fail (except for e.g. out-of-memory).
//   - it is guaranteed to be deterministic.
//   - it will only do one allocation of exactly the right size.
func DeterministicallySerializeAny(apb *anypb.Any) []byte {
	// Preallocate the exact buffer we need.
	allocSize := 0
	if typeURL := apb.GetTypeUrl(); len(typeURL) > 0 {
		typeURLLen := len(typeURL)
		allocSize += len(anyTypeURLTag) + protowire.SizeVarint(uint64(typeURLLen)) + typeURLLen
	}
	if value := apb.GetValue(); len(value) > 0 {
		valueLen := len(value)
		allocSize += len(anyValueTag) + protowire.SizeVarint(uint64(valueLen)) + valueLen
	}
	buf := bytes.NewBuffer(make([]byte, 0, allocSize))

	writeAnyDeterministically(apb, buf)

	return buf.Bytes()
}
