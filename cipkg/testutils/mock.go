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

package testutils

import (
	"fmt"
	"path/filepath"
	"time"

	"go.chromium.org/luci/cipkg/core"
)

var (
	_ core.PackageManager = &MockPackageManager{}
	_ core.PackageHandler = &MockPackageHandler{}
)

// MockPackageManager implements core.PackageManager interface. It stores
// metadata and derivation in the memory. It doesn't allocate any "real" storage
// in the filesystem.
type MockPackageManager struct {
	pkgs    map[string]core.PackageHandler
	baseDir string
}

func NewMockPackageManage(tempDir string) core.PackageManager {
	return &MockPackageManager{
		pkgs:    make(map[string]core.PackageHandler),
		baseDir: tempDir,
	}
}

func (pm *MockPackageManager) Get(id string) core.PackageHandler {
	if h, ok := pm.pkgs[id]; ok {
		return h
	}
	h := &MockPackageHandler{
		id:        id,
		available: false,
		baseDir:   filepath.Join(pm.baseDir, "pkgs", id),
	}
	pm.pkgs[id] = h
	return h
}

type MockPackageHandler struct {
	id        string
	available bool

	lastUsed time.Time
	ref      int

	baseDir string
}

func (p *MockPackageHandler) OutputDirectory() string {
	return filepath.Join(p.baseDir, "content")
}
func (p *MockPackageHandler) LoggingDirectory() string {
	return filepath.Join(p.baseDir, "logs")
}
func (p *MockPackageHandler) Build(f func() error) error {
	if err := f(); err != nil {
		return err
	}
	p.available = true
	return nil
}

func (p *MockPackageHandler) TryRemove() (ok bool, err error) {
	if !p.available || p.ref != 0 {
		return false, nil
	}
	p.available = false
	return true, nil
}

func (p *MockPackageHandler) IncRef() error {
	p.ref += 1
	p.lastUsed = time.Now()
	return nil
}

func (p *MockPackageHandler) DecRef() error {
	if p.ref == 0 {
		return fmt.Errorf("no reference to the package")
	}
	p.ref -= 1
	return nil
}
