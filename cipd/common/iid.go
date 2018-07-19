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

package common

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

// ValidateInstanceID returns an error if the string isn't valid instance id.
func ValidateInstanceID(iid string) (err error) {
	if len(iid) == 40 {
		// Legacy SHA1-based instances use hex(sha1) as instance ID, 40 chars.
		err = checkIsHex(iid)
	} else {
		_, err = decodeObjectRef(iid)
	}
	if err == nil {
		return
	}
	return fmt.Errorf("not a valid package instance ID %q - %s", iid, err)
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

// ObjectRefToInstanceID returns an Instance ID that matches the given CAS
// object ref.
//
// Instance ID is a human readable string representation of ObjectRef used in
// higher level APIs (command line flags, ensure files, etc) and internally in
// the datastore.
//
// Its exact form depends on a hash being used:
//   * SHA1: Instance IDs are hex encoded SHA1 digests, in lowercase.
//   * Everything else: base64(digest + []byte{ref.HashAlgo}), where base64 is
//     without padding and using URL-safe charset.
//
// The ref is not checked for correctness. Use ValidateObjectRef if this is
// a concern. Panics if something is not right.
func ObjectRefToInstanceID(ref *api.ObjectRef) string {
	if err := ValidateObjectRef(ref); err != nil {
		panic(err)
	}
	if ref.HashAlgo == api.HashAlgo_SHA1 {
		return ref.HexDigest // legacy SHA1 instance ID format
	}
	return encodeObjectRef(ref)
}

// InstanceIDToObjectRef is a reverse of ObjectRefToInstanceID.
//
// Panics if the instance ID is incorrect. Use ValidateInstanceID if
// this is a concern.
func InstanceIDToObjectRef(iid string) *api.ObjectRef {
	// Legacy SHA1-based instances use hex(sha1) as instance ID, 40 chars.
	if len(iid) == 40 {
		if err := checkIsHex(iid); err != nil {
			panic(fmt.Errorf("not a valid package instance ID %q - %s", iid, err))
		}
		return &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: iid,
		}
	}
	ref, err := decodeObjectRef(iid)
	if err != nil {
		panic(fmt.Errorf("not a valid package instance ID %q - %s", iid, err))
	}
	return ref
}

////////////////////////////////////////////////////////////////////////////////

// possibleObjRefLen is used to quickly skip wrong IDs in decodeObjectRef.
var possibleObjRefLen map[int]bool

func init() {
	possibleObjRefLen = make(map[int]bool, len(supportedAlgos))
	for algo, prop := range supportedAlgos {
		if prop.hash != nil {
			l := len(encodeObjectRef(&api.ObjectRef{
				HashAlgo:  api.HashAlgo(algo),
				HexDigest: strings.Repeat("0", prop.hexDigestLen),
			}))
			possibleObjRefLen[l] = true
		}
	}
	if possibleObjRefLen[40] {
		panic("oops, there'll be no way to distinguish legacy SHA1 ids from new ones")
	}
}

// encodeObjectRef returns a compact stable human-readable serialization of
// ObjectRef.
//
// It takes the digest blob, appends a byte with ref.HashAlgo value to it, and
// encodes the resulting blob using raw URL-safe base64 encoding.
//
// Panics if ref is invalid.
func encodeObjectRef(ref *api.ObjectRef) string {
	if ref.HashAlgo < 0 || int(ref.HashAlgo) >= len(supportedAlgos) {
		panic(fmt.Errorf("unknown hash algo %d", ref.HashAlgo))
	}
	switch prop := supportedAlgos[ref.HashAlgo]; {
	case prop.hexDigestLen == 0:
		panic(fmt.Errorf("unsupported hash algo %s", ref.HashAlgo))
	case len(ref.HexDigest) != prop.hexDigestLen:
		panic(fmt.Errorf("wrong hex digest len %d for algo %s", len(ref.HexDigest), ref.HashAlgo))
	}

	blob, err := hex.DecodeString(ref.HexDigest)
	if err != nil {
		panic(fmt.Errorf("bad hex digest %q - %s", ref.HexDigest, err))
	}
	blob = append(blob, byte(ref.HashAlgo))
	return base64.RawURLEncoding.EncodeToString(blob)
}

// decodeObjectRef is a reverse of encodeObjectRef.
func decodeObjectRef(iid string) (*api.ObjectRef, error) {
	// Skip obviously wrong instance IDs faster and with a cleaner error message.
	if !possibleObjRefLen[len(iid)] {
		return nil, fmt.Errorf("not a valid size for a digest")
	}

	blob, err := base64.RawURLEncoding.DecodeString(iid)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot base64 decode - %s", err)
	case len(blob) == 0:
		return nil, fmt.Errorf("empty")
	}

	hashAlgo := api.HashAlgo(blob[len(blob)-1])
	digest := blob[:len(blob)-1]

	if int(hashAlgo) >= len(supportedAlgos) {
		return nil, fmt.Errorf("unknown hash algo %d", hashAlgo)
	}
	switch prop := supportedAlgos[hashAlgo]; {
	case prop.hexDigestLen == 0:
		return nil, fmt.Errorf("unsupported hash algo %s", hashAlgo)
	case len(digest)*2 != prop.hexDigestLen:
		return nil, fmt.Errorf("wrong digest len %d for algo %s", len(digest), hashAlgo)
	}

	return &api.ObjectRef{
		HashAlgo:  hashAlgo,
		HexDigest: hex.EncodeToString(digest),
	}, nil
}

// checkIsHex returns an error if a string is not a lowercase hex string.
func checkIsHex(s string) error {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("bad lowercase hex string %q, wrong char %c", s, c)
		}
	}
	return nil
}
