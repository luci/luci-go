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

package local

import (
	"crypto/sha1"
	"encoding/hex"
	"hash"

	"go.chromium.org/luci/cipd/common"
)

// DefaultHash returns a zero hash.Hash instance to use for package verification
// by default.
//
// Currently it is always SHA1.
//
// TODO(vadimsh): Use this function where sha1.New() is used now.
func DefaultHash() hash.Hash {
	return sha1.New()
}

// HashForInstanceID constructs correct zero hash.Hash instance that can be used
// to verify given instance ID.
//
// Currently it is always SHA1.
//
// TODO(vadimsh): Use this function where sha1.New() is used now.
func HashForInstanceID(instanceID string) (hash.Hash, error) {
	if err := common.ValidateInstanceID(instanceID); err != nil {
		return nil, err
	}
	return sha1.New(), nil
}

// InstanceIDFromHash returns an instance ID string, given a hash state.
func InstanceIDFromHash(h hash.Hash) string {
	return hex.EncodeToString(h.Sum(nil))
}
