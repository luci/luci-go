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
	"context"
	"fmt"
	"io"
	"time"

	"github.com/klauspost/compress/gzip"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/config_service/internal/clients"
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

	// ServiceKind is the Datastore entity kind for Service.
	ServiceKind = "Service"

	// CurrentCfgSetVersion is a global version for all ConfigSet entities. It can
	// be used to force a global refresh.
	CurrentCfgSetVersion = 2
)

// ConfigSet is a versioned collection of config files.
type ConfigSet struct {
	_kind string `gae:"$kind,ConfigSetV2"`

	// ID is the name of a config set.
	// Examples: services/luci-config, projects/chromium.
	ID config.Set `gae:"$id"`

	// LatestRevision contains the latest revision info for this ConfigSet.
	LatestRevision RevisionInfo `gae:"latest_revision"`
	// Location is the source location which points the root of this ConfigSet.
	Location *cfgcommonpb.Location `gae:"location"`
	// Version is the global version of the config set.
	// It may be used to decide to force a refresh.
	Version int64 `gae:"version,noindex"`
}

var _ datastore.PropertyLoadSaver = (*ConfigSet)(nil)

// Save implements datastore.PropertyLoadSaver. It makes sure ConfigSet.Version
// always set to CurrentCfgSetVersion when saving into Datastore.
func (cs *ConfigSet) Save(withMeta bool) (datastore.PropertyMap, error) {
	cs.Version = CurrentCfgSetVersion
	return datastore.GetPLS(cs).Save(withMeta)
}

// Load implements datastore.PropertyLoadSaver.
func (cs *ConfigSet) Load(p datastore.PropertyMap) error {
	return datastore.GetPLS(cs).Load(p)
}

// File represents a single config file. Immutable.
//
// TODO(vadimsh): `Content` can be moved to a child entity to allow listing
// file metadata without pulling large blob from the datastore. This will be
// useful in GetConfigSet and DeleteStaleConfigs implementations.
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
	GcsURI gs.Path `gae:"gcs_uri,noindex"`
	// ContentSHA256 is the SHA256 hash of the file content.
	ContentSHA256 string `gae:"content_sha256"`
	// Size is the raw file size in bytes.
	Size int64 `gae:"size,noindex"`
	// Location is a pinned, fully resolved source location to this file.
	Location *cfgcommonpb.Location `gae:"location"`

	// rawContent caches the result of `GetRawContent`. Not saved in datastore.
	rawContent []byte `gae:"-"`
}

// GetPath returns that path to the File.
func (f *File) GetPath() string {
	return f.Path
}

// GetGSPath returns the GCS path to where the config file is stored.
func (f *File) GetGSPath() gs.Path {
	return f.GcsURI
}

// GetRawContent returns the raw and uncompressed content of this config.
//
// May download content from Google Cloud Storage if content is not
// stored inside the entity due to its size.
// The result will be cached so the next GetRawContent call will not pay
// the cost to fetch and decompress.
func (f *File) GetRawContent(ctx context.Context) ([]byte, error) {
	switch {
	case f.rawContent != nil:
		break // rawContent is fetched and cached before.
	case len(f.Content) > 0:
		rawContent, err := decompressData(f.Content)
		if err != nil {
			return nil, err
		}
		f.rawContent = rawContent
	case f.GcsURI != "":
		compressed, err := clients.GetGsClient(ctx).Read(ctx, f.GcsURI.Bucket(), f.GcsURI.Filename(), false)
		if err != nil {
			return nil, errors.Fmt("failed to read from %s: %w", f.GcsURI, err)
		}
		rawContent, err := decompressData(compressed)
		if err != nil {
			return nil, err
		}
		f.rawContent = rawContent
	default:
		return nil, errors.New("both content and gcs_uri are empty")
	}
	return f.rawContent, nil
}

func decompressData(data []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Fmt("failed to create gzip reader: %w", err)
	}
	ret, err := io.ReadAll(gr)
	if err != nil {
		_ = gr.Close()
		return nil, errors.Fmt("failed to decompress the data: %w", err)
	}
	if err := gr.Close(); err != nil {
		return nil, errors.Fmt("errors closing gzip reader: %w", err)
	}
	return ret, nil
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
	// ValidationResult is the result of validating the config set.
	ValidationResult *cfgcommonpb.ValidationResult `gae:"validation_result"`
}

// RevisionInfo contains a revision metadata.
// Referred by ConfigSet and ImportAttempt.
type RevisionInfo struct {
	// ID is a revision name. If imported from Git, it is a commit hash.
	ID string `gae:"id"`
	// Location is a pinned location with revision info in the source repo.
	Location *cfgcommonpb.Location `gae:"location"`
	// CommitTime is the commit time of this revision.
	CommitTime time.Time `gae:"commit_time,noindex"`
	// CommitterEmail is the committer's email.
	CommitterEmail string `gae:"committer_email,noindex"`
	// AuthorEmail is the email of the commit author.
	AuthorEmail string `gae:"author_email,noindex"`
}

// Service contains information about a registered service.
type Service struct {
	// Name is the name of the service.
	Name string `gae:"$id"`
	// Info contains information  for LUCI Config to interact with the service.
	Info *cfgcommonpb.Service `gae:"info"`
	// Metadata describes the metadata of a service.
	Metadata *cfgcommonpb.ServiceMetadata `gae:"metadata"`
	// LegacyMetadata is returned by the service that is still talking in
	// legacy LUCI Config protocol (i.e. REST based).
	//
	// TODO: crbug/1232565 - Remove this support once all backend services are
	// able to talk in the new LUCI Config protocol (i.e. expose
	// `cfgcommonpb.Consumer` interface)
	LegacyMetadata *cfgcommonpb.ServiceDynamicMetadata `gae:"legacy_metadata"`
	// UpdateTime is the time this entity is updated.
	UpdateTime time.Time `gae:"update_time"`
}

// NoSuchConfigError captures the error caused by unknown config set or file.
type NoSuchConfigError struct {
	unknownConfigSet  string
	unknownConfigFile struct {
		configSet, revision, file, hash string
	}
}

// Error implements error interface.
func (e *NoSuchConfigError) Error() string {
	switch {
	case e.unknownConfigSet != "":
		return fmt.Sprintf("can not find config set entity %q from datastore", e.unknownConfigSet)
	case e.unknownConfigFile.file != "":
		return fmt.Sprintf("can not find file entity %q from datastore for config set: %s, revision: %s", e.unknownConfigFile.file, e.unknownConfigFile.configSet, e.unknownConfigFile.revision)
	case e.unknownConfigFile.hash != "":
		return fmt.Sprintf("can not find matching file entity from datastore with hash %q", e.unknownConfigFile.hash)
	default:
		return ""
	}
}

// IsUnknownConfigSet returns true the error is caused by unknown config set.
func (e *NoSuchConfigError) IsUnknownConfigSet() bool {
	return e.unknownConfigSet != ""
}

// IsUnknownFile returns true the error is caused by unknown file name.
func (e *NoSuchConfigError) IsUnknownFile() bool {
	return e.unknownConfigFile.file != ""
}

// IsUnknownFile returns true the error is caused by unknown file hash.
func (e *NoSuchConfigError) IsUnknownFileHash() bool {
	return e.unknownConfigFile.hash != ""
}

// GetLatestConfigFile returns the latest File entity as is for the given
// config set.
//
// Returns NoSuchConfigError when the config set or file can not be found.
func GetLatestConfigFile(ctx context.Context, configSet config.Set, filePath string) (*File, error) {
	cfgSet := &ConfigSet{ID: configSet}
	switch err := datastore.Get(ctx, cfgSet); {
	case err == datastore.ErrNoSuchEntity:
		return nil, &NoSuchConfigError{unknownConfigSet: string(configSet)}
	case err != nil:
		return nil, errors.Fmt("failed to fetch ConfigSet %q: %w", configSet, err)
	}
	f := &File{
		Path:     filePath,
		Revision: datastore.MakeKey(ctx, ConfigSetKind, string(configSet), RevisionKind, cfgSet.LatestRevision.ID),
	}
	switch err := datastore.Get(ctx, f); {
	case err == datastore.ErrNoSuchEntity:
		return nil, &NoSuchConfigError{
			unknownConfigFile: struct {
				configSet, revision, file, hash string
			}{
				configSet: f.Revision.Root().StringID(),
				revision:  f.Revision.StringID(),
				file:      f.Path,
			}}
	case err != nil:
		return nil, errors.Fmt("failed to fetch file %q: %w", f.Path, err)
	}
	return f, nil
}

// GetConfigFileByHash fetches a file entity by content hash for the given
// config set. If multiple file entities are found, the most recently created
// one will be returned.
//
// Returns NoSuchConfigError when the matching file can not be found in the
// storage.
func GetConfigFileByHash(ctx context.Context, configSet config.Set, contentSha256 string) (*File, error) {
	var latestFile *File
	err := datastore.Run(ctx, datastore.NewQuery(FileKind).Eq("content_sha256", contentSha256), func(file *File) error {
		if file.Revision.Root().StringID() == string(configSet) &&
			(latestFile == nil || file.CreateTime.After(latestFile.CreateTime)) {
			latestFile = file
		}
		return nil
	})
	switch {
	case err != nil:
		return nil, errors.Fmt("failed to query file by sha256 hash %q: %w", contentSha256, err)
	case latestFile == nil:
		return nil, &NoSuchConfigError{
			unknownConfigFile: struct {
				configSet, revision, file, hash string
			}{hash: contentSha256}}
	}
	return latestFile, nil
}
