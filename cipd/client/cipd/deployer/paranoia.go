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

package deployer

import (
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/common/cipderr"
)

// ParanoidMode specifies how paranoid CIPD client should be.
type ParanoidMode string

const (
	// NotParanoid indicates that CIPD client should trust its metadata
	// directory: if a package is marked as installed there, it should be
	// considered correctly installed in the site root too.
	NotParanoid ParanoidMode = "NotParanoid"

	// CheckPresence indicates that CIPD client should verify all files
	// that are supposed to be installed into the site root are indeed present
	// there.
	//
	// Note that it will not check file's content or file mode. Only its presence.
	CheckPresence ParanoidMode = "CheckPresence"

	// CheckIntegrity indicates that CIPD client should verify all files installed
	// in the site root have correct content (based on their hashes).
	//
	// CIPD will use information from 'stat' to skip rechecking hashes all the
	// time. Only files modified (based on 'stat') since they were installed are
	// checked.
	CheckIntegrity ParanoidMode = "CheckIntegrity"
)

// Validate returns an error if the mode is unrecognized.
func (p ParanoidMode) Validate() error {
	switch p {
	case NotParanoid, CheckPresence, CheckIntegrity:
		return nil
	default:
		return cipderr.BadArgument.Apply(errors.Fmt("unrecognized paranoid mode %q", p))
	}
}
