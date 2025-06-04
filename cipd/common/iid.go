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

	"go.chromium.org/luci/common/errors"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

// HashAlgoValidation is passed to ValidateObjectRef.
type HashAlgoValidation bool

const (
	// KnownHash indicates that ValidateObjectRef should only accept refs that
	// use hash algo known to the current version of the code.
	//
	// This is primarily useful on the backend, since it always needs to be sure
	// that it can handle any ObjectRef that is passed to it.
	KnownHash HashAlgoValidation = true

	// AnyHash indicates that ValidateObject may accept refs that use algo number
	// not known to the current version of the code (i.e. presumably from the
	// future version of the protocol).
	//
	// This is useful on the client that for some operations just round-trips
	// ObjectRef it received from the server, without trying to interpret it.
	// Using AnyHash allows the client to be friendlier to future protocol
	// changes. This is particularly important in self-update flow.
	AnyHash HashAlgoValidation = false
)

// ValidateInstanceID returns an error if the string isn't a valid instance id.
//
// If v is KnownHash, will verify the current version of the code understands
// the hash algo encoded in idd. Otherwise (if v is AnyHash) will just validate
// that iid looks syntactically sane, and will accept any hash algo (even if the
// current code doesn't understand it). This is useful when round-tripping
// ObjectRefs and instance IDs through the (outdated) client code back to the
// server.
func ValidateInstanceID(iid string, v HashAlgoValidation) (err error) {
	if len(iid) == 40 {
		// Legacy SHA1-based instances use hex(sha1) as instance ID, 40 chars.
		err = checkIsHex(iid)
		// Refuse SHA1s if the binary doesn't have SHA1 support compiled in.
		if err == nil && v == KnownHash {
			err = ValidateHashAlgo(api.HashAlgo_SHA1)
		}
	} else {
		var ref *api.ObjectRef
		ref, err = decodeObjectRef(iid)
		if err == nil && v == KnownHash {
			err = ValidateObjectRef(ref, KnownHash)
		}
	}
	if err == nil {
		return
	}
	return errors.Fmt("not a valid package instance ID %q: %w", iid, err)
}

// ValidateObjectRef returns a grpc-annotated error if the given object ref is
// invalid.
//
// If v is KnownHash, will verify the current version of the code understands
// the hash algo. Otherwise (if v is AnyHash) will just validate that the hex
// digest looks sane, and will accept any hash algo (even if the current code
// doesn't understand it). This is useful when round-tripping ObjectRefs and
// instance IDs through the (outdated) client code back to the server.
//
// Errors have InvalidArgument grpc code.
func ValidateObjectRef(ref *api.ObjectRef, v HashAlgoValidation) error {
	if ref == nil {
		return validationErr("the object ref is not provided")
	}

	switch {
	case ref.HashAlgo < 0:
		return validationErr("bad negative hash algo")
	case ref.HashAlgo == 0:
		return validationErr("unspecified hash algo")
	}

	if err := checkIsHex(ref.HexDigest); err != nil {
		return errors.Fmt("invalid %s hex digest: %w", ref.HashAlgo, err)
	}

	if v {
		if err := ValidateHashAlgo(ref.HashAlgo); err != nil {
			return err
		}
		hexDigestLen := supportedAlgos[ref.HashAlgo].hexDigestLen
		if len(ref.HexDigest) != hexDigestLen {
			return validationErr("invalid %s digest: expecting %d chars, got %d", ref.HashAlgo, hexDigestLen, len(ref.HexDigest))
		}
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
//   - SHA1: Instance IDs are hex encoded SHA1 digests, in lowercase.
//   - Everything else: base64(digest + []byte{ref.HashAlgo}), where base64 is
//     without padding and using URL-safe charset.
//
// The ref is not checked for correctness. Use ValidateObjectRef if this is
// a concern. Panics if something is not right.
func ObjectRefToInstanceID(ref *api.ObjectRef) string {
	if err := ValidateObjectRef(ref, AnyHash); err != nil {
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
			panic(fmt.Errorf("not a valid package instance ID %q: %s", iid, err))
		}
		return &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: iid,
		}
	}
	ref, err := decodeObjectRef(iid)
	if err != nil {
		panic(fmt.Errorf("not a valid package instance ID %q: %s", iid, err))
	}
	return ref
}

////////////////////////////////////////////////////////////////////////////////

// encodeObjectRef returns a compact stable human-readable serialization of
// ObjectRef.
//
// It takes the digest blob, appends a byte with ref.HashAlgo value to it, and
// encodes the resulting blob using raw URL-safe base64 encoding.
//
// Panics if ref is invalid.
func encodeObjectRef(ref *api.ObjectRef) string {
	switch {
	case ref.HashAlgo < 0:
		panic(fmt.Errorf("bad negative hash algo %d", ref.HashAlgo))
	case ref.HashAlgo == 0:
		panic(fmt.Errorf("unspecified hash algo"))
	}

	// If algo is known to us, make sure it is valid. Otherwise just encode what
	// we've given, assuming the consumer will eventually understand it.
	if int(ref.HashAlgo) < len(supportedAlgos) {
		if prop := supportedAlgos[ref.HashAlgo]; len(ref.HexDigest) != prop.hexDigestLen {
			panic(fmt.Errorf("wrong hex digest len %d for algo %s", len(ref.HexDigest), ref.HashAlgo))
		}
	}

	blob, err := hex.DecodeString(ref.HexDigest)
	if err != nil {
		panic(fmt.Errorf("bad hex digest %q: %s", ref.HexDigest, err))
	}
	blob = append(blob, byte(ref.HashAlgo))
	return base64.RawURLEncoding.EncodeToString(blob)
}

// decodeObjectRef is a reverse of encodeObjectRef.
func decodeObjectRef(iid string) (*api.ObjectRef, error) {
	// Skip obviously wrong instance IDs faster and with a cleaner error message.
	// We assume we use at least 160 bit digests here (which translates to at
	// least 28 bytes of encoded iid).
	if len(iid) < 28 {
		return nil, validationErr("not a valid size for an encoded digest")
	}

	blob, err := base64.RawURLEncoding.DecodeString(iid)
	switch {
	case err != nil:
		return nil, validationErr("cannot base64 decode: %s", err)
	case len(blob) == 0:
		return nil, validationErr("empty")
	case len(blob)%2 != 1: // 1 byte for hashAlgo, the rest is the digest
		return nil, validationErr("the digest can't be odd")
	}

	hashAlgo := api.HashAlgo(blob[len(blob)-1])
	digest := blob[:len(blob)-1]

	if hashAlgo == 0 {
		return nil, validationErr("unspecified hash algo (0)")
	}

	// If algo is known to us, make sure it is valid. Otherwise just decode what
	// we've given, assuming the caller will later verify the hash using
	// ValidateHashAlgo, if really needed.
	if int(hashAlgo) < len(supportedAlgos) {
		if prop := supportedAlgos[hashAlgo]; len(digest)*2 != prop.hexDigestLen {
			return nil, validationErr("wrong digest len %d for algo %s", len(digest), hashAlgo)
		}
	}

	return &api.ObjectRef{
		HashAlgo:  hashAlgo,
		HexDigest: hex.EncodeToString(digest),
	}, nil
}

// checkIsHex returns an error if a string is not a lowercase hex string.
//
// Empty string is rejected as invalid too.
func checkIsHex(s string) error {
	switch {
	case s == "":
		return validationErr("empty hex string")
	case len(s)%2 != 0:
		return validationErr("uneven number of symbols in the hex string")
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return validationErr("bad lowercase hex string %q, wrong char %c", s, c)
		}
	}
	return nil
}
