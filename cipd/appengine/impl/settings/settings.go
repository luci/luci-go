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
//
// These are settings that are usually set only once after the initial service
// deployment. They are exposed through Admin portal interface.
package settings

import (
	"context"
	"flag"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/portal"
	"go.chromium.org/luci/server/settings"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/gs"
)

const settingsKey = "cipd"

// Settings contain CIPD backend settings.
type Settings struct {
	// StorageGSPath is GS path in a form of /bucket/path to the root of the
	// content-addressable storage area in Google Storage.
	//
	// The files will be stored as /storage_gs_path/hash_algo/hex_digest.
	StorageGSPath string `json:"storage_gs_path"`

	// TempGSPath is GS path in a form of /bucket/path to the root of the storage
	// area for pending uploads.
	//
	// It contains unverified files uploaded by clients before they pass the
	// hash verification check and copied to the CAS storage area.
	TempGSPath string `json:"temp_gs_path"`
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
		return errors.Reason("-cipd-storage-gs-path is required").Err()
	}
	if err := gs.ValidatePath(s.StorageGSPath); err != nil {
		return errors.Annotate(err, "bad -cipd-storage-gs-path").Err()
	}
	if s.TempGSPath == "" {
		return errors.Reason("-cipd-temp-gs-path is required").Err()
	}
	if err := gs.ValidatePath(s.TempGSPath); err != nil {
		return errors.Annotate(err, "bad -cipd-temp-gs-path").Err()
	}
	return nil
}

// Get returns the settings from the local cache, checking they are populated.
//
// Returns grpc-annotated Internal error if something is wrong.
func Get(ctx context.Context) (*Settings, error) {
	s := &Settings{}
	if err := settings.Get(ctx, settingsKey, s); err != nil && err != settings.ErrNoSettings {
		return nil, errors.Annotate(err, "failed to fetch settings").
			Tag(grpcutil.InternalTag, transient.Tag).Err()
	}
	if s.StorageGSPath == "" || s.TempGSPath == "" {
		return nil, errors.Reason("the backend is not configured").
			Tag(grpcutil.InternalTag).Err()
	}
	return s, nil
}

// ObjectPath constructs a path to the object in the Google Storage, starting
// from StorageGSPath root.
func (s *Settings) ObjectPath(obj *api.ObjectRef) string {
	return s.StorageGSPath + "/" + obj.HashAlgo.String() + "/" + obj.HexDigest
}

type settingsPage struct {
	portal.BasePage
}

func (*settingsPage) Title(context.Context) (string, error) {
	return "CIPD settings", nil
}

func (*settingsPage) Fields(context.Context) ([]portal.Field, error) {
	return []portal.Field{
		{
			ID:    "StorageGSPath",
			Title: "Content store path",
			Type:  portal.FieldText,
			Help: "<p>A Google Storage path in a form of /bucket/path to the root of the " +
				"content-addressable storage area in Google Storage. The files will be " +
				"stored as /storage_gs_path/hash_algo/hex_digest.</p>",
			Placeholder: "/<bucket>/<path>",
			Validator: func(p string) error {
				if p == "" {
					return nil
				}
				return gs.ValidatePath(p)
			},
		},
		{
			ID:    "TempGSPath",
			Title: "Temp path",
			Type:  portal.FieldText,
			Help: "<p>A Google Storage path in a form of /bucket/path to the root of the " +
				"storage area for pending uploads. It contains unverified files uploaded " +
				"by clients before they pass the hash verification check and copied to " +
				"the CAS storage area.</p>",
			Placeholder: "/<bucket>/<path>",
			Validator: func(p string) error {
				if p == "" {
					return nil
				}
				return gs.ValidatePath(p)
			},
		},
	}, nil
}

func (*settingsPage) ReadSettings(ctx context.Context) (map[string]string, error) {
	c := &Settings{}
	if err := settings.GetUncached(ctx, settingsKey, c); err != nil && err != settings.ErrNoSettings {
		return nil, err
	}
	return map[string]string{
		"StorageGSPath": c.StorageGSPath,
		"TempGSPath":    c.TempGSPath,
	}, nil
}

func (*settingsPage) WriteSettings(ctx context.Context, values map[string]string, who, why string) error {
	return settings.SetIfChanged(ctx, settingsKey, &Settings{
		StorageGSPath: values["StorageGSPath"],
		TempGSPath:    values["TempGSPath"],
	}, who, why)
}

func init() {
	portal.RegisterPage(settingsKey, &settingsPage{})
}
