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

package internal

import (
	"bytes"
	"crypto/sha256"
	"errors"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/cipd/client/cipd/internal/messages"
)

// ErrUnknownSHA256 indicates the deserialized message doesn't have SHA256 set.
//
// This can happen when deserializing records in the old format.
var ErrUnknownSHA256 = errors.New("no sha256 is recorded in the file")

// MarshalWithSHA256 serializes proto message to bytes, calculates SHA256
// checksum of it, and returns serialized envelope that contains both.
//
// UnmarshalWithSHA256 can then be used to verify SHA256 and deserialized the
// original object.
func MarshalWithSHA256(pm proto.Message) ([]byte, error) {
	blob, err := proto.Marshal(pm)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(blob)
	envelope := messages.BlobWithSHA256{Blob: blob, Sha256: sum[:]}
	return proto.Marshal(&envelope)
}

// UnmarshalWithSHA256 is reverse of MarshalWithSHA256.
//
// It checks SHA256 checksum and deserializes the object if it matches the blob.
//
// If the expected SHA256 is not available in 'buf', returns ErrUnknownSHA256.
// This can happen when reading blobs in old format that used SHA1.
func UnmarshalWithSHA256(buf []byte, pm proto.Message) error {
	envelope := messages.BlobWithSHA256{}
	if err := proto.Unmarshal(buf, &envelope); err != nil {
		return err
	}
	if len(envelope.Sha256) == 0 {
		return ErrUnknownSHA256
	}
	sum := sha256.Sum256(envelope.Blob)
	if !bytes.Equal(sum[:], envelope.Sha256) {
		return errors.New("sha256 of the file is invalid, it is probably corrupted")
	}
	return proto.Unmarshal(envelope.Blob, pm)
}
