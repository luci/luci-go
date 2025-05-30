// Copyright 2015 The LUCI Authors.
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

package flex

import (
	"context"
	"time"

	gcst "cloud.google.com/go/storage"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/storage/archive"
	"go.chromium.org/luci/logdog/common/storage/bigtable"
)

const (
	// maxSignedURLLifetime is the maximum allowed signed URL lifetime.
	maxSignedURLLifetime = 1 * time.Hour
)

// Services is a set of support services used by Coordinator endpoints.
//
// Each instance is valid for a single request, but can be re-used throughout
// that request. This is advised, as the Services instance may optionally cache
// values.
//
// Services methods are goroutine-safe.
type Services interface {
	// Storage returns a Storage instance for the supplied log stream.
	//
	// The caller must close the returned instance if successful.
	StorageForStream(ctx context.Context, state *coordinator.LogStreamState, project string) (coordinator.SigningStorage, error)
}

// GlobalServices is an application singleton that stores cross-request service
// structures.
//
// It lives in the root context.
type GlobalServices struct {
	btStorage       *bigtable.Storage
	gsClientFactory func(ctx context.Context, project string) (gs.Client, error)
	storageCache    *StorageCache
}

// NewGlobalServices instantiates a new GlobalServices instance.
//
// Receives the location of the BigTable with intermediate logs.
//
// The Context passed to GlobalServices should be a global server Context not a
// request-specific Context.
func NewGlobalServices(ctx context.Context, bt *bigtable.Flags) (*GlobalServices, error) {
	// LRU in-memory cache in front of BigTable.
	storageCache := &StorageCache{}

	// Construct the storage, inject the caching implementation into it.
	storage, err := bigtable.StorageFromFlags(ctx, bt)
	if err != nil {
		return nil, errors.Fmt("failed to connect to BigTable: %w", err)
	}
	storage.Cache = storageCache

	return &GlobalServices{
		btStorage:    storage,
		storageCache: storageCache,
		gsClientFactory: func(ctx context.Context, project string) (client gs.Client, e error) {
			// TODO(vadimsh): Switch to AsProject + WithProject(project) once
			// we are ready to roll out project scoped service accounts in Logdog.
			transport, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
			if err != nil {
				return nil, errors.Fmt("failed to create Google Storage RPC transport: %w", err)
			}
			prodClient, err := gs.NewProdClient(ctx, transport)
			if err != nil {
				return nil, errors.Fmt("Failed to create GS client.: %w", err)
			}
			return prodClient, nil
		},
	}, nil
}

// Storage returns a Storage instance for the supplied log stream.
//
// The caller must close the returned instance if successful.
func (gsvc *GlobalServices) StorageForStream(ctx context.Context, lst *coordinator.LogStreamState, project string) (
	coordinator.SigningStorage, error) {

	if !lst.ArchivalState().Archived() {
		logging.Debugf(ctx, "Log is not archived. Fetching from intermediate storage.")
		return noSignedURLStorage{gsvc.btStorage}, nil
	}

	// Some very old logs have malformed data where they claim to be archived but
	// have no archive or index URLs.
	if lst.ArchiveStreamURL == "" {
		logging.Warningf(ctx, "Log has no archive URL")
		return nil, grpcutil.NotFoundTag.Apply(errors.New("log has no archive URL"))
	}
	if lst.ArchiveIndexURL == "" {
		logging.Warningf(ctx, "Log has no index URL")
		return nil, grpcutil.NotFoundTag.Apply(errors.New("log has no index URL"))
	}

	gsClient, err := gsvc.gsClientFactory(ctx, project)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to create Google Storage client.")
		return nil, err
	}

	logging.Fields{
		"indexURL":    lst.ArchiveIndexURL,
		"streamURL":   lst.ArchiveStreamURL,
		"archiveTime": lst.ArchivedTime,
	}.Debugf(ctx, "Log is archived. Fetching from archive storage.")

	st, err := archive.New(archive.Options{
		Index:  gs.Path(lst.ArchiveIndexURL),
		Stream: gs.Path(lst.ArchiveStreamURL),
		Cache:  gsvc.storageCache,
		Client: gsClient,
	})
	if err != nil {
		logging.WithError(err).Errorf(ctx, "Failed to create Google Storage storage instance.")
		return nil, err
	}

	rv := &googleStorage{
		Storage: st,
		svc:     gsvc,
		gs:      gsClient,
		stream:  gs.Path(lst.ArchiveStreamURL),
		index:   gs.Path(lst.ArchiveIndexURL),
	}
	return rv, nil
}

// noSignedURLStorage is a thin wrapper around a Storage instance that cannot
// sign URLs.
type noSignedURLStorage struct {
	storage.Storage
}

func (noSignedURLStorage) GetSignedURLs(context.Context, *coordinator.URLSigningRequest) (
	*coordinator.URLSigningResponse, error) {

	return nil, nil
}

type googleStorage struct {
	// Storage is the base storage.Storage instance.
	storage.Storage
	// svc is the services instance that created this.
	svc *GlobalServices

	// gs is the backing Google Storage client.
	gs gs.Client

	// stream is the stream's Google Storage URL.
	stream gs.Path
	// index is the index's Google Storage URL.
	index gs.Path
}

func (si *googleStorage) Close() {
	si.Storage.Close()
	si.gs.Close()
}

func (si *googleStorage) GetSignedURLs(ctx context.Context, req *coordinator.URLSigningRequest) (*coordinator.URLSigningResponse, error) {
	signer := auth.GetSigner(ctx)
	info, err := signer.ServiceInfo(ctx)
	if err != nil {
		return nil, errors.Fmt("failed to get service info: %w", err)
	}

	lifetime := req.Lifetime
	switch {
	case lifetime < 0:
		return nil, errors.Fmt("invalid signed URL lifetime: %s", lifetime)

	case lifetime > maxSignedURLLifetime:
		lifetime = maxSignedURLLifetime
	}

	// Get our signing options.
	resp := coordinator.URLSigningResponse{
		Expiration: clock.Now(ctx).Add(lifetime),
	}
	opts := gcst.SignedURLOptions{
		GoogleAccessID: info.ServiceAccountName,
		SignBytes: func(b []byte) ([]byte, error) {
			_, signedBytes, err := signer.SignBytes(ctx, b)
			return signedBytes, err
		},
		Method:  "GET",
		Expires: resp.Expiration,
	}

	doSign := func(path gs.Path) (string, error) {
		url, err := gcst.SignedURL(path.Bucket(), path.Filename(), &opts)
		if err != nil {
			logging.Warningf(ctx, "failed to sign URL: bucket(%s)/filename(%s)", path.Bucket(), path.Filename())
			return "", errors.Fmt("failed to sign URL: %w", err)
		}
		return url, nil
	}

	// Sign stream URL.
	if req.Stream {
		if resp.Stream, err = doSign(si.stream); err != nil {
			return nil, errors.Fmt("failed to sign stream URL: %w", err)
		}
	}

	// Sign index URL.
	if req.Index {
		if resp.Index, err = doSign(si.index); err != nil {
			return nil, errors.Fmt("failed to sign index URL: %w", err)
		}
	}

	return &resp, nil
}
