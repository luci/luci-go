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

// Package settings contains definition of global CIPD backend settings.
package settings

import (
	"flag"

	"go.chromium.org/luci/common/errors"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
)

// Settings contain CIPD backend settings.
type Settings struct {
	// StorageGSPath is GS path in a form of /bucket/path to the root of the
	// content-addressable storage area in Google Storage.
	//
	// The files will be stored as /storage_gs_path/hash_algo/hex_digest.
	StorageGSPath string

	// TempGSPath is GS path in a form of /bucket/path to the root of the storage
	// area for pending uploads.
	//
	// It contains unverified files uploaded by clients before they pass the
	// hash verification check and copied to the CAS storage area.
	TempGSPath string
}

// Register registers settings as CLI flags.
func (s *Settings) Register(f *flag.FlagSet) {
	f.StringVar(
		&s.StorageGSPath,
		"cipd-storage-gs-path",
		s.StorageGSPath,
		"The root of the content-addressable storage area in Google Storage as a '/bucket/path' string.",
	)
	f.StringVar(
		&s.TempGSPath,
		"cipd-temp-gs-path",
		s.TempGSPath,
		"The root of the pending uploads storage area in Google Storage as a '/bucket/path' string.",
	)
}

// Validate validates settings format.
func (s *Settings) Validate() error {
	if s.StorageGSPath == "" {
		return errors.New("-cipd-storage-gs-path is required")
	}
	if err := gs.ValidatePath(s.StorageGSPath); err != nil {
		return errors.Fmt("bad -cipd-storage-gs-path: %w", err)
	}
	if s.TempGSPath == "" {
		return errors.New("-cipd-temp-gs-path is required")
	}
	if err := gs.ValidatePath(s.TempGSPath); err != nil {
		return errors.Fmt("bad -cipd-temp-gs-path: %w", err)
	}
	return nil
}

// ObjectPath constructs a path to the object in the Google Storage, starting
// from StorageGSPath root.
func (s *Settings) ObjectPath(obj *caspb.ObjectRef) string {
	return s.StorageGSPath + "/" + obj.HashAlgo.String() + "/" + obj.HexDigest
}
