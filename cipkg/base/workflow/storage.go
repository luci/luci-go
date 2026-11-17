// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package workflow

import (
	"context"
	"errors"
	"path"
	"slices"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/core"
)

// RemoteStorage represents a remote storage layer for fetching and uploading
// packages.
// Fetch should return core.ErrPackageNotExist if the package is not found in
// remote storage. Both Fetch and Upload may return an optional content hash.
type RemoteStorage interface {
	Fetch(ctx context.Context, pkg actions.Package, id string) (hash string, err error)
	Upload(ctx context.Context, pkg actions.Package, ids []string) (hash string, err error)
}

var _ RemoteStorage = (*ReadOnlyStorage)(nil)

// ReadOnlyStorage is a RemoteStorage middleware that forwards Fetch to the
// underlying RemoteStorage while making Upload a no-op.
type ReadOnlyStorage struct {
	inner RemoteStorage
}

// NewReadOnlyStorage creates a ReadOnlyStorage wrapping inner.
func NewReadOnlyStorage(inner RemoteStorage) *ReadOnlyStorage {
	return &ReadOnlyStorage{inner: inner}
}

// Fetch fetches pkg from the underlying RemoteStorage.
func (r *ReadOnlyStorage) Fetch(ctx context.Context, pkg actions.Package, id string) (string, error) {
	return r.inner.Fetch(ctx, pkg, id)
}

// Upload is a no-op for ReadOnlyStorage.
func (r *ReadOnlyStorage) Upload(context.Context, actions.Package, []string) (string, error) {
	return "", nil
}

var _ RemoteStorage = (*RelocatableStorage)(nil)

// RelocatableStorage is a RemoteStorage middleware that resolves a derivation ID
// generated with a fixed storage path, which can be used as a stable ID for a
// relocatable package.
type RelocatableStorage struct {
	processor *actions.ActionProcessor
	inner     RemoteStorage
}

// NewRelocatableStorage creates a RelocatableStorage using the provided
// ActionProcessor and underlying RemoteStorage.
func NewRelocatableStorage(ap *actions.ActionProcessor, inner RemoteStorage) *RelocatableStorage {
	return &RelocatableStorage{
		processor: ap,
		inner:     inner,
	}
}

func (r *RelocatableStorage) resolveID(pkg actions.Package) (string, error) {
	p, err := r.processor.Process(pkg.Derivation.GetPlatform(), relocatableStubPM, pkg.Action)
	if err != nil {
		return "", err
	}
	return p.DerivationID, nil
}

// Fetch resolves the relocatable ID for pkg and fetches it from the underlying
// RemoteStorage.
func (r *RelocatableStorage) Fetch(ctx context.Context, pkg actions.Package, id string) (string, error) {
	if id == "" {
		var err error
		if id, err = r.resolveID(pkg); err != nil {
			return "", err
		}
	}
	return r.inner.Fetch(ctx, pkg, id)
}

// Upload resolves the relocatable ID for pkg and uploads it to the underlying
// RemoteStorage.
func (r *RelocatableStorage) Upload(ctx context.Context, pkg actions.Package, ids []string) (string, error) {
	id, err := r.resolveID(pkg)
	if err != nil {
		return "", err
	}
	return r.inner.Upload(ctx, pkg, append(slices.Clone(ids), id))
}

var relocatableStubPM = newStubPackageManager("/cipkg/v1")

type stubPackageManager struct {
	storagePath string
}

// newStubPackageManager creates a PackageManager with a fixed storage path
// that does not touch the filesystem. It is used to calculate reproducible
// cache IDs for relocatable packages.
func newStubPackageManager(storagePath string) core.PackageManager {
	return &stubPackageManager{storagePath: storagePath}
}

func (pm *stubPackageManager) Get(id string) core.PackageHandler {
	return &stubPackageHandler{
		baseDirectory: path.Join(pm.storagePath, id),
	}
}

type stubPackageHandler struct {
	baseDirectory string
}

func (h *stubPackageHandler) OutputDirectory() string {
	return path.Join(h.baseDirectory, "contents")
}

func (h *stubPackageHandler) LoggingDirectory() string {
	return path.Join(h.baseDirectory, "logs")
}

func (h *stubPackageHandler) Build(func() error) error {
	return errors.New("stub package handler cannot build")
}

func (h *stubPackageHandler) TryRemove() (bool, error) {
	return false, nil
}

func (h *stubPackageHandler) IncRef() error {
	return nil
}

func (h *stubPackageHandler) DecRef() error {
	return nil
}
