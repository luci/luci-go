// Copyright 2023 The LUCI Authors.
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

// Package model package model contains Datastore models Config Service uses.
package model

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"time"

	"go.chromium.org/luci/common/errors"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
)

const (
	// ConfigSetKind is the Datastore entity kind for ConfigSet.
	ConfigSetKind = "ConfigSetV2"

	// RevisionKind is the Datastore entity kind for Revision.
	RevisionKind = "RevisionV2"

	// FileKind is the Datastore entity kind for File.
	FileKind = "FileV2"

	// ImportAttemptKind is the Datastore entity kind for ImportAttempt.
	ImportAttemptKind = "ImportAttemptV2"
)

// ConfigSet is a versioned collection of config files.
type ConfigSet struct {
	_kind string `gae:"$kind,ConfigSetV2"`

	// ID is the name of a config set.
	// Examples: services/luci-config, projects/chromium.
	ID config.Set `gae:"$id"`

	// LatestRevision contains the latest revision info for this ConfigSet.
	LatestRevision RevisionInfo `gae:"latest_revision,noindex"`
	// Location is the source location which points the root of this ConfigSet.
	Location *cfgcommonpb.Location `gae:"location"`
	// Version is the global version of the config set.
	// It may be used to decide to force a refresh.
	Version int64 `gae:"version,noindex"`
}

// File represents a single config file. Immutable
type File struct {
	_kind string `gae:"$kind,FileV2"`

	//  Path is the file path relative to its config set root path.
	Path string `gae:"$id"`
	// Revision is a key for parent Revision.
	Revision *datastore.Key `gae:"$parent"`
	// CreateTime is the timestamp when this File entity is imported.
	CreateTime time.Time `gae:"create_time,noindex"`
	// Content is the gzipped raw content of the small config file.
	Content []byte `gae:"content,noindex"`
	// GcsURI is a Google Cloud Storage URI where it stores large gzipped file.
	// The format is "gs://<bucket>/<object_name>"
	// Note: Either Content field or GcsUri field will be set, but not both.
	GcsURI string `gae:"gcs_uri,noindex"`
	// ContentHash is the SHA256 hash of the file content.
	ContentHash string `gae:"content_hash"`
	// Location is a pinned, fully resolved source location to this file.
	Location *cfgcommonpb.Location `gae:"location"`
}

// ImportAttempt describes what happened last time we tried to import a config
// set.
type ImportAttempt struct {
	_kind string `gae:"$kind,ImportAttemptV2"`

	// ID is always the string "last" because we only need last attempt info.
	ID string `gae:"$id,last"`

	// ConfigSet is a key for parent ConfigSet.
	ConfigSet *datastore.Key `gae:"$parent"`
	// Revision refers to the revision info.
	Revision RevisionInfo `gae:"revision,noindex"`
	// Success indicates whether this attempt is succeeded.
	Success bool `gae:"success,noindex"`
	// Message is a human-readable message about this import attempt.
	Message string `gae:"message,noindex"`
	// ValidationMessage is the error return by the corresponding downstream
	// application when calling its validation API.
	ValidationMessage *cfgcommonpb.ValidationResponseMessage `gae:"validationMessage"`
}

// RevisionInfo contains a revision metadata.
// Referred by ConfigSet and ImportAttempt.
type RevisionInfo struct {
	// ID is a revision name. If imported from Git, it is a commit hash.
	ID string `gae:"id"`
	// Location is a pinned location with revision info in the source repo.
	Location *cfgcommonpb.Location `gae:"location"`
	// CommitTime is the commit time of this revision.
	CommitTime time.Time `gae:"time"`
	// CommitterEmail is the committer's email.
	CommitterEmail string `gae:"committer_email"`
}

// GetLatestConfigFile returns the latest File for the given config set.
// If resolveGcsURI, it will download content from File.GcsURI.
func GetLatestConfigFile(ctx context.Context, configSet config.Set, filePath string, resolveGcsURI bool) (*File, error) {
	cfgSet := &ConfigSet{ID: configSet}
	if err := datastore.Get(ctx, cfgSet); err != nil {
		return nil, errors.Annotate(err, "failed to fetch ConfigSet %q", configSet).Err()
	}
	file := &File{
		Path:     filePath,
		Revision: datastore.MakeKey(ctx, ConfigSetKind, string(configSet), RevisionKind, cfgSet.LatestRevision.ID),
	}
	if err := file.Load(ctx, resolveGcsURI); err != nil {
		return nil, errors.Annotate(err, "failed to fetch file %q in %q", file.Path, configSet).Err()
	}
	return file, nil
}

// Load loads the file entity from Datastore with content being decompressed.
// If resolveGcsURI is true, it will download content from File.GcsURI.
func (f *File) Load(ctx context.Context, resolveGcsURI bool) error {
	switch {
	case f.Path != "" && f.Revision != nil:
		if err := datastore.Get(ctx, f); err != nil {
			return err
		}
	case f.ContentHash != "":
		found := false
		if err := datastore.Run(ctx, datastore.NewQuery(FileKind).Eq("content_hash", f.ContentHash), func(file *File) error {
			*f = *file
			found = true
			return datastore.Stop
		}); err != nil {
			return err
		}
		if !found {
			return datastore.ErrNoSuchEntity
		}
	default:
		return errors.Reason("One of ContentHash or (path and revision) is required").Err()
	}

	//  TODO(crbug.com/1446839): download from GCS if resolveGcsURI is true

	// For empty file, the gzipped content bytes will still be non-nil.
	if len(f.Content) == 0 {
		return errors.Reason("file content is nil. Might be damaged?").Err()
	}
	gr, err := gzip.NewReader(bytes.NewReader(f.Content))
	if err != nil {
		return errors.Annotate(err, "failed to create gzip reader").Err()
	}
	uncompressed, err := io.ReadAll(gr)
	if err != nil {
		return errors.Annotate(err, "failed to uncompress file content").Err()
	}
	f.Content = uncompressed
	return nil
}
