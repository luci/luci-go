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
	"fmt"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
)

// ObjectRefToInstanceID returns an Instance ID that matches the given CAS
// object ref.
//
// Instance ID is a human readable string representation of ObjectRef used in
// higher level APIs (command line flags, ensure files, etc) and internally in
// the datastore.
//
// Its exact form depends on a hash being used:
//   * SHA1: Instance IDs are hex encoded SHA1 digests, in lowercase.
//   * SHA256: Instance IDs are URL-safe raw base64 encoded SHA256 digests.
//
// The ref is not checked for correctness. Use ValidateObjectRef if this is
// a concern. Panics if something is not right.
func ObjectRefToInstanceID(ref *api.ObjectRef) string {
	if err := ValidateObjectRef(ref); err != nil {
		panic(err)
	}
	iid, err := supportedAlgos[ref.HashAlgo].hexDigestToIID(ref.HexDigest)
	if err != nil {
		panic(err)
	}
	return iid
}

// InstanceIDToObjectRef is a reverse of ObjectRefToInstanceID.
//
// It doesn't check the instance ID for correctness. Use ValidateInstanceID if
// this is a concern. Panics if something is not right.
func InstanceIDToObjectRef(iid string) *api.ObjectRef {
	algo := algoByIIDLen[len(iid)]
	if algo == 0 {
		panic(fmt.Errorf("not a valid package instance ID %q - wrong length", iid))
	}
	props := supportedAlgos[algo]

	switch hex, err := props.iidToHexDigest(iid); {
	case err != nil:
		panic(fmt.Errorf("not a valid package instance ID %q - %s", iid, err))
	case len(hex) != props.hexDigestLen:
		panic(fmt.Errorf("after parsing %q, hex length %d != %d", iid, len(hex), props.hexDigestLen))
	default:
		return &api.ObjectRef{
			HashAlgo:  algo,
			HexDigest: hex,
		}
	}
}
