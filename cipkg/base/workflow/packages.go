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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/filesystem"

	"github.com/danjacques/gofslock/fslock"
)

// LocalPackageManager implements core.PackageManager.
var _ core.PackageManager = &LocalPackageManager{}

// LocalPackageManager is a PackageManager implementation that stores packages
// locally. It supports recording package references acrossing multiple
// instances using fslock.
type LocalPackageManager struct {
	storagePath string
}

func NewLocalPackageManager(path string) (*LocalPackageManager, error) {
	if err := os.MkdirAll(path, fs.ModePerm); err != nil {
		return nil, fmt.Errorf("initialize local storage failed: %s: %w", path, err)
	}
	s := &LocalPackageManager{
		storagePath: path,
	}
	return s, nil
}

// Get returns the handler for the package.
func (pm *LocalPackageManager) Get(id string) core.PackageHandler {
	return &localPackageHandler{
		baseDirectory: filepath.Join(pm.storagePath, id),
		lockFile:      filepath.Join(pm.storagePath, fmt.Sprintf(".%s.lock", id)),
	}
}

// Prune try remove packages if:
// - Not available, or
// - Haven't been used for `ttl` time.
func (pm *LocalPackageManager) Prune(c context.Context, ttl time.Duration, max int) {
	deadline := time.Now().Add(-ttl)
	locks, err := fs.Glob(os.DirFS(pm.storagePath), ".*.lock")
	if err != nil {
		logging.WithError(err).Warningf(c, "failed to list locks")
	}
	pruned := 0
	for _, l := range locks {
		id := l[1 : len(l)-5] // remove prefix "." and suffix ".lock"
		pkg := pm.Get(id).(*localPackageHandler)
		if t := pkg.lastUsed(); ttl == 0 || t.Before(deadline) {
			if removed, err := pkg.TryRemove(); err != nil {
				logging.WithError(err).Warningf(c, "failed to remove package")
			} else if removed {
				logging.Debugf(c, "prune: remove package (not used since %s): %s", t, id)
				if pruned++; pruned == max {
					logging.Debugf(c, "prune: hit prune limit of %d ", max)
					break
				}
			}
		} else {
			logging.Debugf(c, "prune: skip package (not used since %s): %s", t, id)
		}
	}
}

type localPackageHandler struct {
	baseDirectory string
	lockFile      string
	rlockHandle   fslock.Handle
}

func (h *localPackageHandler) OutputDirectory() string {
	return filepath.Join(h.baseDirectory, "contents")
}

func (h *localPackageHandler) LoggingDirectory() string {
	return filepath.Join(h.baseDirectory, "logs")
}

func (h *localPackageHandler) Build(builder func() error) error {
	if h.rlockHandle != nil {
		return fmt.Errorf("can't build package when read lock is held")
	}

	var errPotentialRaceCondition = errors.New("potential race condition; retry with shared lock")

	blocker := func() error {
		// Avoid a race case:
		// +-----------+             +-------+                +-----------+
		// | vpython1  |             | lock  |                | vpython2  |
		// +-----------+             +-------+                +-----------+
		//       |                       |                          |
		//       | Require Exclusive     |        Require Exclusive |
		//       |---------------------->|<-------------------------|
		//       |                       |                          |
		//       |     Acquire Exclusive | -------------------\     |
		//       |<----------------------|-| Lock unavailable |     |
		//       |                       | |------------------|     |
		//       | Build Package         |                          |
		//       |--------------         |                          |
		//       |             |         |                          |
		//       |<-------------         |                          |
		//       |                       |                          |
		//       | Release Exclusive     | -----------------\       |
		//       |---------------------->|-| Lock available |       |
		//       |                       | |----------------|       |
		//       | Require Shared        |                          |
		//       |---------------------->|                          |
		//       |                       |                          |
		//       |        Acquire Shared | -------------------\     |
		//       |<----------------------|-| Lock unavailable |     |
		//       |                       | |------------------|     |
		//       | Executing Python      |                          |
		//       |-----------------      |                          |
		//       |                |      |                          |
		//       |<----------------      |                          |
		//       |                       |                          |
		//       | Release Shared        | -----------------\       |
		//       |---------------------->|-| Lock available |       |
		//       |                       | |----------------|       |
		//       |                       |                          |
		//       |                       | Acquire Exclusive        |
		//       |                       |------------------------->|
		//       |                       |                          |
		// If vpython2 miss the time windows from Release Exclusive to Acquire
		// Shared for acquiring exclusive lock, it will wait the lock forever until
		// vpython1 exit.
		//
		// In this case, we can check stamp and if its available, probably other
		// process is/was holding the exclusive lock and building the package. Try
		// shared lock instead so Build() can return as soon as exclusive lock
		// released.
		if t := h.lastUsed(); !t.IsZero() {
			return errPotentialRaceCondition
		}

		time.Sleep(time.Millisecond * 10)
		return nil
	}

	if err := fslock.WithBlocking(h.lockFile, blocker, func() error {
		if t := h.lastUsed(); !t.IsZero() {
			return nil
		}

		if err := filesystem.RemoveAll(h.baseDirectory); err != nil {
			return fmt.Errorf("failed to remove package dir: %s: %w", h.baseDirectory, err)
		}
		if err := os.MkdirAll(h.OutputDirectory(), fs.ModePerm); err != nil {
			return fmt.Errorf("failed to create package dir: %s: %w", h.OutputDirectory(), err)
		}

		if err := builder(); err != nil {
			return fmt.Errorf("failed to execute builder: %w", err)
		}

		f, err := os.Create(h.stampPath())
		if err != nil {
			return fmt.Errorf("failed to create stamp file: %s: %w", h.stampPath(), err)
		}
		defer f.Close()
		if err := h.touch(); err != nil {
			return fmt.Errorf("failed to touch stamp file: %s: %w", h.stampPath(), err)
		}
		return nil
	}); errors.Is(errPotentialRaceCondition, err) {
		// Although there is still a small chance that package (stamp) being removed
		// and should back to waiting exclusive lock for rebuild, we want to avoid
		// unexpected infinite loop here. Return error if we failed to acquire shared
		// lock.
		if err := h.IncRef(); err != nil {
			return fmt.Errorf("failed to acquire read lock after potential race condition: %w", err)
		}
		return h.DecRef()
	} else {
		return err
	}
}

func (h *localPackageHandler) TryRemove() (ok bool, err error) {
	switch err := fslock.With(h.lockFile, func() error {
		if err := filesystem.RemoveAll(h.baseDirectory); err != nil {
			return fmt.Errorf("failed to remove package dir: %s: %w", h.baseDirectory, err)
		}
		return nil
	}); err {
	case nil:
		if err := filesystem.RemoveAll(h.lockFile); err != nil {
			return false, nil
		}
		return true, nil
	case fslock.ErrLockHeld:
		return false, nil
	default:
		return false, err
	}
}

func (h *localPackageHandler) lastUsed() time.Time {
	if s, err := os.Stat(h.stampPath()); err == nil {
		return s.ModTime()
	}

	return time.Time{}
}

func (h *localPackageHandler) IncRef() error {
	if h.rlockHandle != nil {
		return fmt.Errorf("acquire read lock multiple times on same package")
	}

	blocker := func() error {
		time.Sleep(time.Millisecond * 10)
		return nil
	}

	rl, err := fslock.LockSharedBlocking(h.lockFile, blocker)
	if err != nil {
		return fmt.Errorf("failed to acquire read lock: %w", err)
	}
	if err := func() error {
		if err := rl.PreserveExec(); err != nil {
			return fmt.Errorf("failed to perserve lock: %w", err)
		}
		if t := h.lastUsed(); t.IsZero() {
			return core.ErrPackageNotExist
		}

		// Update mtime of the stamp since at this point we ensured:
		// 1. Package is locked and won't be removed
		// 2. Stamp is presented in the package
		if err := h.touch(); err != nil {
			return fmt.Errorf("failed to touch the the stamp: %w", err)
		}
		return nil
	}(); err != nil {
		// Release the shared lock if any error happens.
		return errors.Join(err, rl.Unlock())
	}
	h.rlockHandle = rl
	return nil
}

func (h *localPackageHandler) DecRef() error {
	if err := h.rlockHandle.Unlock(); err != nil {
		return fmt.Errorf("failed to release read lock: %w", err)
	}
	h.rlockHandle = nil
	return nil
}

func (h *localPackageHandler) stampPath() string {
	return filepath.Join(h.baseDirectory, "stamp")
}

func (h *localPackageHandler) touch() error {
	return filesystem.Touch(h.stampPath(), time.Time{}, 0644)
}

// MustIncRefRecursiveRuntime will IncRef the package with all its runtime
// dependencies recursively. If an error happened, it may panic with only
// part of the packages are referenced.
func MustIncRefRecursiveRuntime(pkg actions.Package) {
	if err := doPackageRecursiveRuntime(pkg, make(map[string]struct{}),
		func(pkg actions.Package) error { return pkg.Handler.IncRef() }); err != nil {
		panic(err)
	}
}

// MustDecRefRecursiveRuntime will DecRef the package with all its runtime
// dependencies recursively. If an error happened, it may panic with only
// part of the packages are dereferenced.
func MustDecRefRecursiveRuntime(pkg actions.Package) {
	if err := doPackageRecursiveRuntime(pkg, make(map[string]struct{}),
		func(pkg actions.Package) error { return pkg.Handler.DecRef() }); err != nil {
		panic(err)
	}
}

func doPackageRecursiveRuntime(pkg actions.Package, visited map[string]struct{}, f func(pkg actions.Package) error) error {
	if _, ok := visited[pkg.DerivationID]; ok {
		return nil
	}
	if err := f(pkg); err != nil {
		return err
	}
	visited[pkg.DerivationID] = struct{}{}

	for _, d := range pkg.RuntimeDependencies {
		if err := doPackageRecursiveRuntime(d, visited, f); err != nil {
			return err
		}
	}
	return nil
}
