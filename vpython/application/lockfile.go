// Copyright 2026 The LUCI Authors.
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

package application

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/vpython/common"
	"go.chromium.org/luci/vpython/standard"
)

// SyncLockfile ensures the attached <specPath>.uv.lock file is synchronized
// with the spec source and exports the frozen dependencies to the runtime.
func SyncLockfile(ctx context.Context, specPath, uvBin, pythonBin string, isBot bool, spec *standard.ProjectSpec) error {
	lockPath := specPath + ".uv.lock"

	_, err := os.Stat(specPath)
	if err != nil {
		return errors.Fmt("failed to stat spec source: %w", err)
	}

	outOfSync := false
	var missingDeps []string
	lockData, readErr := os.ReadFile(lockPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}

	// Fast synchronization check: ensure all base packages requested in the spec
	// are present in the lockfile, regardless of version strings.
	lockedNames := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(lockData))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		lockedNames[extractBaseName(line)] = true
	}

	for _, dep := range spec.Dependencies {
		baseName := extractBaseName(dep)
		if !lockedNames[baseName] {
			missingDeps = append(missingDeps, baseName)
			outOfSync = true
		}
	}

	if outOfSync {
		var reason string
		if len(missingDeps) > 0 {
			reason = fmt.Sprintf("missing dependencies: %s", strings.Join(missingDeps, ", "))
		} else {
			reason = "file is missing"
		}

		if isBot {
			if readErr != nil && errors.Is(readErr, os.ErrNotExist) {
				return errors.Fmt("locked environment file %s is missing in project CWD! Bots must strictly execute locked dependencies.", filepath.Base(lockPath))
			}
			return errors.Fmt("locked environment file %s is out-of-sync with spec source in project CWD (%s). Developers must run 'vpython3' locally to synchronize and commit their lockfile changes.", filepath.Base(lockPath), reason)
		}

		var err error
		lockData, err = updateLockfile(ctx, specPath, lockPath, uvBin, pythonBin, spec, reason)
		if err != nil {
			return err
		}
	}

	var frozenDeps []string
	scanner = bufio.NewScanner(bytes.NewReader(lockData))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		line = strings.TrimSuffix(line, "\\")
		line = strings.TrimSpace(line)
		if line != "" {
			frozenDeps = append(frozenDeps, line)
		}
	}

	spec.Dependencies = frozenDeps
	logging.Infof(ctx, "Successfully resolved %d frozen dependencies (with markers) from %s!", len(frozenDeps), filepath.Base(lockPath))
	return nil
}

func updateLockfile(ctx context.Context, specPath, lockPath, uvBin, pythonBin string, spec *standard.ProjectSpec, reason string) ([]byte, error) {
	// Developer mode: synchronize the attached lockfile locally via stdin.
	fmt.Printf("vpython3: %s is missing or out-of-sync (%s). Synchronizing lockfile on-the-fly...\n", filepath.Base(lockPath), reason)
	logging.Infof(ctx, "%s is missing or out-of-sync (%s). Synchronizing via uv pip compile...", filepath.Base(lockPath), reason)

	var reqs bytes.Buffer
	for _, dep := range spec.Dependencies {
		reqs.WriteString(dep)
		reqs.WriteByte('\n')
	}

	cmdLock := exec.CommandContext(ctx, uvBin, "pip", "compile", "-",
		"--universal",
		"--generate-hashes",
		"--no-header",
	)
	cmdLock.Dir = filepath.Dir(specPath)
	cmdLock.Stdin = &reqs

	arURL := os.Getenv(common.EnvVpythonArUrl)
	if arURL == "" {
		// Will be replaced with common source in the next CL.
		arURL = "https://us-python.pkg.dev/chrome-python-ar/chrome-python-ar/simple/"
	}
	env := append(os.Environ(),
		"UV_PYTHON_DOWNLOADS=never",
		"UV_NO_WORKSPACE=1",
		"UV_PYTHON="+pythonBin,
	)
	if arURL != "" {
		env = append(env, "UV_DEFAULT_INDEX="+arURL)
	}
	cmdLock.Env = env

	var stdout, stderr bytes.Buffer
	cmdLock.Stdout = &stdout
	cmdLock.Stderr = &stderr

	if err := cmdLock.Run(); err != nil {
		return nil, errors.Fmt("failed to compile lockfile %s: %s\nOutput:\n%s", filepath.Base(lockPath), err, stderr.String())
	}

	if err := os.WriteFile(lockPath, stdout.Bytes(), 0644); err != nil {
		return nil, errors.Fmt("failed to write lockfile %s: %w", filepath.Base(lockPath), err)
	}

	fmt.Printf("vpython3: Successfully synchronized lockfile %s!\n", filepath.Base(lockPath))
	logging.Infof(ctx, "Successfully synchronized lockfile %s!", filepath.Base(lockPath))

	return stdout.Bytes(), nil
}

// extractBaseName extracts the base package name from a PEP 508 dependency string.
func extractBaseName(dep string) string {
	idx := strings.IndexAny(dep, " [=<>~;@")
	if idx != -1 {
		dep = dep[:idx]
	}
	return normalizeBaseName(dep)
}

// normalizeBaseName normalizes a Python package name for comparison.
func normalizeBaseName(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, ".", "-")
	return s
}
