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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
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

// Supported algo => its properties, a factory function and converters.
//
// All iidLen MUST be different. We use them to differentiate between different
// hash algos when parsing IIDs in ValidateInstanceID and InstanceIDToObjectRef.
var supportedAlgos = []*struct {
	hash           func() hash.Hash
	hexDigestLen   int                              // len of hex(digest blob) string
	iidLen         int                              // len of IID produced by hexDigestToIID
	hexDigestToIID func(hex string) (string, error) // note: when called, len(hex) == hexDigestLen
	iidToHexDigest func(iid string) (string, error) // note: when called, len(iid) == iidLen
}{
	api.HashAlgo_HASH_ALGO_UNSPECIFIED: {},

	api.HashAlgo_SHA1: {
		hash:         sha1.New,
		hexDigestLen: 40,
		iidLen:       40,
		hexDigestToIID: func(h string) (string, error) {
			return h, nil
		},
		iidToHexDigest: func(iid string) (string, error) {
			if err := checkIsHex(iid); err != nil {
				return "", err
			}
			return iid, nil
		},
	},

	api.HashAlgo_SHA256: {
		hash:         sha256.New,
		hexDigestLen: 64,
		iidLen:       43,
		hexDigestToIID: func(h string) (string, error) {
			blob, err := hex.DecodeString(h)
			if err != nil {
				return "", err
			}
			return base64.RawURLEncoding.EncodeToString(blob), nil
		},
		iidToHexDigest: func(iid string) (string, error) {
			blob, err := base64.RawURLEncoding.DecodeString(iid)
			if err != nil {
				return "", err
			}
			return hex.EncodeToString(blob), nil
		},
	},
}

// Maps len(instance ID string) to api.HashAlgo used to derive it.
var algoByIIDLen map[int]api.HashAlgo

func init() {
	algoByIIDLen = make(map[int]api.HashAlgo, len(supportedAlgos)-1)
	for i := 1; i < len(supportedAlgos); i++ { // skip UNSPECIFIED
		props := supportedAlgos[i]
		if algoByIIDLen[props.iidLen] != 0 {
			panic(fmt.Sprintf("can't have two hashes with same IID length %d", props.iidLen))
		}
		algoByIIDLen[props.iidLen] = api.HashAlgo(i)
	}
}

func checkIsHex(s string) error {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("bad lowercase hex string %q, wrong char %c", s, c)
		}
	}
	return nil
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
	case int(h) >= len(supportedAlgos) || supportedAlgos[h].hexDigestLen == 0:
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

	hexDigestLen := supportedAlgos[ref.HashAlgo].hexDigestLen
	if len(ref.HexDigest) != hexDigestLen {
		return errors.Reason("invalid %s digest: expecting %d chars, got %d", ref.HashAlgo, hexDigestLen, len(ref.HexDigest)).
			Tag(grpcutil.InvalidArgumentTag).Err()
	}

	if err := checkIsHex(ref.HexDigest); err != nil {
		return errors.Annotate(err, "invalid %s digest", ref.HashAlgo).
			Tag(grpcutil.InvalidArgumentTag).Err()
	}

	return nil
}

// HexDigest returns a digest string as it is used in ObjectRef.
func HexDigest(h hash.Hash) string {
	return hex.EncodeToString(h.Sum(nil))
}
