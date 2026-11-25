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
	"sync"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/core"
)

// RemoteStorage represents a remote storage layer for resolving, fetching, and
// uploading packages.
// Resolve and Fetch should return core.ErrPackageNotExist if the package is not
// found in remote storage. Resolve, Fetch, and Upload may return an optional
// content hash.
type RemoteStorage interface {
	Resolve(ctx context.Context, pkg actions.Package, id string) (hash string, err error)
	Fetch(ctx context.Context, pkg actions.Package, id string) (hash string, err error)
	Upload(ctx context.Context, pkg actions.Package, ids []string) (hash string, err error)
}

var _ RemoteStorage = (*ReadOnlyStorage)(nil)

// ReadOnlyStorage is a RemoteStorage middleware that forwards Resolve and Fetch
// to the underlying RemoteStorage while making Upload a no-op.
type ReadOnlyStorage struct {
	inner RemoteStorage
}

// NewReadOnlyStorage creates a ReadOnlyStorage wrapping inner.
func NewReadOnlyStorage(inner RemoteStorage) *ReadOnlyStorage {
	return &ReadOnlyStorage{inner: inner}
}

// Resolve resolves pkg in the underlying RemoteStorage.
func (r *ReadOnlyStorage) Resolve(ctx context.Context, pkg actions.Package, id string) (string, error) {
	return r.inner.Resolve(ctx, pkg, id)
}

// Fetch fetches pkg from the underlying RemoteStorage.
func (r *ReadOnlyStorage) Fetch(ctx context.Context, pkg actions.Package, id string) (string, error) {
	return r.inner.Fetch(ctx, pkg, id)
}

// Upload is a no-op for ReadOnlyStorage.
func (r *ReadOnlyStorage) Upload(context.Context, actions.Package, []string) (string, error) {
	return "", nil
}

var _ RemoteStorage = (*StableStorage)(nil)

// StableStorage is a RemoteStorage middleware that resolves stable derivation
// IDs for relocatable packages using a fixed storage path and, when available,
// content hashes of their build dependencies.
type StableStorage struct {
	processor *actions.ActionProcessor
	inner     RemoteStorage
	hashes    sync.Map
}

// NewStableStorage creates a StableStorage using the provided ActionProcessor
// and underlying RemoteStorage.
func NewStableStorage(ap *actions.ActionProcessor, inner RemoteStorage) *StableStorage {
	return &StableStorage{
		processor: ap,
		inner:     inner,
	}
}

func (s *StableStorage) resolvePkg(ctx context.Context, buildPlat string, p actions.Package) (string, actions.Package, error) {
	if v, ok := s.hashes.Load(p.DerivationID); ok {
		return v.(string), p, nil
	}

	hash, err := s.inner.Resolve(ctx, p, p.DerivationID)
	switch {
	case err == nil:
		if hash != "" {
			s.hashes.Store(p.DerivationID, hash)
		}
		return hash, p, nil
	case !errors.Is(err, core.ErrPackageNotExist):
		return "", actions.Package{}, err
	}

	transformed, err := s.transformDeps(ctx, buildPlat, p)
	if err != nil || transformed.DerivationID == p.DerivationID {
		return "", p, err
	}

	hash, err = s.inner.Resolve(ctx, p, transformed.DerivationID)
	switch {
	case err == nil:
		if hash != "" {
			s.hashes.Store(p.DerivationID, hash)
		}
		return hash, transformed, nil
	case errors.Is(err, core.ErrPackageNotExist):
		return "", transformed, nil
	default:
		return "", actions.Package{}, err
	}
}

func (s *StableStorage) transformDeps(ctx context.Context, buildPlat string, p actions.Package) (actions.Package, error) {
	if len(p.BuildDependencies) == 0 {
		return p, nil
	}

	deps := make([]actions.Package, len(p.BuildDependencies))
	for i, d := range p.BuildDependencies {
		hash, resolved, err := s.resolvePkg(ctx, buildPlat, d)
		if err != nil {
			return actions.Package{}, err
		}
		if hash != "" {
			fixedDrv := &core.Derivation{
				Name:        d.Action.GetName(),
				FixedOutput: hash,
			}
			cid, err := core.GetDerivationID(fixedDrv)
			if err != nil {
				return actions.Package{}, err
			}
			d.Derivation = fixedDrv
			d.DerivationID = cid
			d.Handler = stableStubPM.Get(cid)
			deps[i] = d
		} else {
			deps[i] = resolved
		}
	}

	drv, err := s.processor.Transform(ctx, buildPlat, p.Action, deps)
	if err != nil {
		return actions.Package{}, err
	}
	cid, err := core.GetDerivationID(drv)
	if err != nil {
		return actions.Package{}, err
	}
	p.Derivation = drv
	p.DerivationID = cid
	p.Handler = stableStubPM.Get(cid)
	return p, nil
}

// Resolve resolves stable IDs for pkg and checks it in the underlying
// RemoteStorage without downloading package contents.
func (s *StableStorage) Resolve(ctx context.Context, pkg actions.Package, id string) (string, error) {
	if id != "" {
		return s.inner.Resolve(ctx, pkg, id)
	}
	buildPlat := pkg.Derivation.GetPlatform()
	relocPkg, err := s.processor.Process(ctx, buildPlat, stableStubPM, pkg.Action)
	if err != nil {
		return "", err
	}
	hash, _, err := s.resolvePkg(ctx, buildPlat, relocPkg)
	if err != nil {
		return "", err
	}
	if hash == "" {
		return "", core.ErrPackageNotExist
	}
	return hash, nil
}

// Fetch resolves stable IDs for pkg and fetches it from the underlying
// RemoteStorage.
func (s *StableStorage) Fetch(ctx context.Context, pkg actions.Package, id string) (string, error) {
	if id != "" {
		return s.inner.Fetch(ctx, pkg, id)
	}

	buildPlat := pkg.Derivation.GetPlatform()
	relocPkg, err := s.processor.Process(ctx, buildPlat, stableStubPM, pkg.Action)
	if err != nil {
		return "", err
	}
	relocID := relocPkg.DerivationID

	hash, err := s.inner.Fetch(ctx, pkg, relocID)
	switch {
	case err == nil:
		if hash != "" {
			s.hashes.Store(relocID, hash)
		}
		return hash, nil
	case !errors.Is(err, core.ErrPackageNotExist):
		return "", err
	}

	transformed, err := s.transformDeps(ctx, buildPlat, relocPkg)
	if err != nil {
		return "", err
	}
	if transformed.DerivationID == relocID {
		return "", core.ErrPackageNotExist
	}

	hash, err = s.inner.Fetch(ctx, pkg, transformed.DerivationID)
	if err == nil && hash != "" {
		s.hashes.Store(relocID, hash)
	}
	return hash, err
}

// Upload resolves stable IDs for pkg and uploads it to the underlying
// RemoteStorage.
func (s *StableStorage) Upload(ctx context.Context, pkg actions.Package, ids []string) (string, error) {
	buildPlat := pkg.Derivation.GetPlatform()
	relocPkg, err := s.processor.Process(ctx, buildPlat, stableStubPM, pkg.Action)
	if err != nil {
		return "", err
	}
	relocID := relocPkg.DerivationID

	transformed, err := s.transformDeps(ctx, buildPlat, relocPkg)
	if err != nil {
		return "", err
	}

	out := slices.Clone(ids)
	if transformed.DerivationID != relocID {
		out = append(out, transformed.DerivationID)
	}
	out = append(out, relocID)

	hash, err := s.inner.Upload(ctx, pkg, out)
	if err != nil {
		return "", err
	}
	if hash != "" {
		s.hashes.Store(relocID, hash)
	}
	return hash, nil
}

var stableStubPM = newStubPackageManager("/cipkg/v1")

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
