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
	"runtime"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/environ"

	"go.chromium.org/luci/vpython/common"
	"go.chromium.org/luci/vpython/python"
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

	if len(spec.Dependencies) == 0 {
		return nil
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

// DefaultSupportedPythonVersions lists the CPython major.minor versions bundled with vpython3.
var DefaultSupportedPythonVersions = []string{"3.8", "3.11"}

// resolveTargetPythonVersion resolves a PEP 440 requires-python constraint
// against vpython's bundled Python versions, defaulting to the highest
// bundled version ("3.11") when requiresPython is empty.
func resolveTargetPythonVersion(requiresPython string) (string, error) {
	ver, err := standard.MatchInterpreter(requiresPython, DefaultSupportedPythonVersions)
	if err != nil {
		return "", errors.Fmt("failed to resolve compatible Python interpreter for constraint %q: %w", requiresPython, err)
	}
	return ver, nil
}

// resolveBundledPythonBin probes for the bundled CPython interpreter matching
// targetVer relative to the running vpython3 executable.
func resolveBundledPythonBin(targetVer string) string {
	execDir, err := python.FindExecutableDir()
	if err != nil {
		return ""
	}
	pythonBundle := filepath.Join(execDir, common.DefaultBundleDir(targetVer))
	candidates := []string{filepath.Join(pythonBundle, "bin", "python3")}
	if runtime.GOOS == "windows" {
		candidates = []string{
			filepath.Join(pythonBundle, "bin", "python.exe"),
			filepath.Join(pythonBundle, "python.exe"),
		}
	}
	for _, cand := range candidates {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	return ""
}

// compileLockfile invokes `uv pip compile` to resolve universal dependencies
// bounded to the target Python version from spec.RequiresPython, and writes
// the resulting lockfile to lockPath.
func compileLockfile(ctx context.Context, specPath, lockPath, uvBin, pythonBin string, spec *standard.ProjectSpec) ([]byte, error) {
	targetVer, err := resolveTargetPythonVersion(spec.RequiresPython)
	if err != nil {
		return nil, err
	}

	// Create an isolated temporary project with [tool.uv] environments bounded to
	// targetVer. Without this, `uv pip compile --universal --python-version X.Y`
	// treats X.Y only as a lower bound (>=X.Y) with no upper bound, attempting to
	// resolve wheels for future Python versions (e.g., cp313, >=3.14) that may not
	// exist in the wheelhouse mirror.
	tmpProjDir, err := os.MkdirTemp("", "vpython-uv-proj-*")
	if err != nil {
		return nil, errors.Fmt("failed to create temporary uv project directory: %w", err)
	}
	defer os.RemoveAll(tmpProjDir)

	pyprojectContent := fmt.Sprintf(
		"[project]\nname = \"vpython-env\"\nversion = \"0.0.0\"\nrequires-python = \"==%s.*\"\n\n[tool.uv]\nenvironments = [\"python_version == '%s'\"]\n",
		targetVer, targetVer,
	)
	if err := os.WriteFile(filepath.Join(tmpProjDir, "pyproject.toml"), []byte(pyprojectContent), 0644); err != nil {
		return nil, errors.Fmt("failed to write temporary pyproject.toml: %w", err)
	}

	var reqs bytes.Buffer
	for _, dep := range spec.Dependencies {
		reqs.WriteString(dep)
		reqs.WriteByte('\n')
	}

	cmdLock := exec.CommandContext(ctx, uvBin, "pip", "compile", "-",
		"--universal",
		"--generate-hashes",
		"--no-header",
		"--python-version", targetVer,
		"--project", tmpProjDir,
	)
	cmdLock.Dir = filepath.Dir(specPath)
	cmdLock.Stdin = &reqs

	if pythonBin == "" {
		pythonBin = resolveBundledPythonBin(targetVer)
	}

	arURL := os.Getenv(common.EnvVpythonArUrl)
	if arURL == "" {
		arURL = common.DefaultARURL
	}
	env := environ.System()
	env.Set("UV_PYTHON_DOWNLOADS", "never")
	env.Set("UV_NO_WORKSPACE", "1")
	if pythonBin != "" {
		env.Set("UV_PYTHON", pythonBin)
	} else {
		env.Remove("UV_PYTHON")
	}
	if arURL != "" {
		env.Set("UV_DEFAULT_INDEX", arURL)
	}
	cmdLock.Env = env.Sorted()

	var stdout, stderr bytes.Buffer
	cmdLock.Stdout = &stdout
	cmdLock.Stderr = &stderr

	if err := cmdLock.Run(); err != nil {
		return nil, errors.Fmt("failed to compile lockfile %s: %s\nOutput:\n%s", filepath.Base(lockPath), err, stderr.String())
	}

	if err := os.WriteFile(lockPath, stdout.Bytes(), 0644); err != nil {
		return nil, errors.Fmt("failed to write lockfile %s: %w", filepath.Base(lockPath), err)
	}

	return stdout.Bytes(), nil
}

func updateLockfile(ctx context.Context, specPath, lockPath, uvBin, pythonBin string, spec *standard.ProjectSpec, reason string) ([]byte, error) {
	// Developer mode: synchronize the attached lockfile locally via stdin.
	logging.Infof(ctx, "%s is missing or out-of-sync (%s). Synchronizing via uv pip compile...", filepath.Base(lockPath), reason)

	lockData, err := compileLockfile(ctx, specPath, lockPath, uvBin, pythonBin, spec)
	if err != nil {
		return nil, err
	}

	logging.Infof(ctx, "Successfully synchronized lockfile %s!", filepath.Base(lockPath))

	return lockData, nil
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
