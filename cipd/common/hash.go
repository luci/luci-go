// Copyright 2017 The LUCI Authors.
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

package common

import (
	"crypto/sha1"
	"encoding/hex"
	"hash"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

// DefaultHashAlgo is a hash algorithm to use for deriving IDs of new package
// instances.
//
// Currently SHA1, but will change to SHA256 soon. Older existing instances are
// allowed to use some other hash algo.
const DefaultHashAlgo = api.HashAlgo_SHA1

// Supported algo => its digest length (in hex encoding) + factory function.
var supportedAlgos = []struct {
	digestLen int
	hash      func() hash.Hash
}{
	api.HashAlgo_HASH_ALGO_UNSPECIFIED: {},
	api.HashAlgo_SHA1:                  {40, sha1.New},
}

// NewHash returns a hash implementation or an error if the algo is unknown.
func NewHash(algo api.HashAlgo) (hash.Hash, error) {
	if err := ValidateHashAlgo(algo); err != nil {
		return nil, err
	}
	return supportedAlgos[algo].hash(), nil
}

// MustNewHash as like NewHash, but panics on errors.
//
// Appropriate for cases when the hash algo has already been validated.
func MustNewHash(algo api.HashAlgo) hash.Hash {
	h, err := NewHash(algo)
	if err != nil {
		panic(err)
	}
	return h
}

// ValidateHashAlgo returns a grpc-annotated error if the given algo is invalid.
//
// Errors have InvalidArgument grpc code.
func ValidateHashAlgo(h api.HashAlgo) error {
	switch {
	case h == api.HashAlgo_HASH_ALGO_UNSPECIFIED:
		return errors.Reason("the hash algorithm is not specified").
			Tag(grpcutil.InvalidArgumentTag).Err()
	case int(h) >= len(supportedAlgos) || supportedAlgos[h].digestLen == 0:
		return errors.Reason("unsupported hash algorithm %d", h).
			Tag(grpcutil.InvalidArgumentTag).Err()
	}
	return nil
}

// ValidateObjectRef returns a grpc-annotated error if the given object ref is
// invalid.
//
// Errors have InvalidArgument grpc code.
func ValidateObjectRef(ref *api.ObjectRef) error {
	if ref == nil {
		return errors.Reason("the object ref is not provided").
			Tag(grpcutil.InvalidArgumentTag).Err()
	}

	if err := ValidateHashAlgo(ref.HashAlgo); err != nil {
		return err
	}

	digestLen := supportedAlgos[ref.HashAlgo].digestLen
	if len(ref.HexDigest) != digestLen {
		return errors.Reason("invalid %s digest: expecting %d chars, got %d", ref.HashAlgo, digestLen, len(ref.HexDigest)).
			Tag(grpcutil.InvalidArgumentTag).Err()
	}

	for _, c := range ref.HexDigest {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return errors.Reason("invalid %s digest: wrong char %c", ref.HashAlgo, c).
				Tag(grpcutil.InvalidArgumentTag).Err()
		}
	}

	return nil
}

// HexDigest returns a digest string as it is used in ObjectRef.
func HexDigest(h hash.Hash) string {
	return hex.EncodeToString(h.Sum(nil))
}
