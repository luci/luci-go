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
	"errors"
	"path"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/core"
)

// RelocatableCacheID generates a derivation id with a stable storage path,
// which can be used as the cache id for a relocatable package.
func RelocatableCacheID(buildPlat string, ap *actions.ActionProcessor, a *core.Action) (string, error) {
	pkg, err := ap.Process(buildPlat, relocatableStubPM, a)
	if err != nil {
		return "", err
	}
	return pkg.DerivationID, nil
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
