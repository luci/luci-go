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
// Instance IDs of packages that use SHA1 refs are just hex encoded SHA1
// digests, in lowercase.
//
// The ref is not checked for correctness. Use ValidateObjectRef if this is
// a concern. Panics if something is not right.
func ObjectRefToInstanceID(ref *api.ObjectRef) string {
	switch ref.HashAlgo {
	case api.HashAlgo_SHA1:
		return ref.HexDigest
	default:
		panic(fmt.Sprintf("unrecognized hash algo %d", ref.HashAlgo))
	}
}

// InstanceIDToObjectRef is a reverse of ObjectRefToInstanceID.
//
// It doesn't check the instance ID for correctness. Use ValidateInstanceID if
// this is a concern. Panics if something is not right.
func InstanceIDToObjectRef(iid string) *api.ObjectRef {
	ref := &api.ObjectRef{
		HashAlgo:  api.HashAlgo_SHA1, // TODO(vadimsh): Recognize more.
		HexDigest: iid,
	}
	if err := ValidateObjectRef(ref); err != nil {
		panic(fmt.Sprintf("bad instance ID %q, the resulting ref is broken - %s", iid, err))
	}
	return ref
}
