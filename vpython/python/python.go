// Copyright 2022 The LUCI Authors.
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

// Package python includes generating python venv and parsing python command
// line arguments.
package python

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/base/workflow"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/system/environ"

	"go.chromium.org/luci/vpython/common"
	"go.chromium.org/luci/vpython/standard"
)

type Environment struct {
	Executable string
	CPython    generators.Generator
	Virtualenv generators.Generator
}

func CPythonFromCIPD(version string) generators.Generator {
	return &generators.CIPDExport{
		Name: "cpython",
		Ensure: ensure.File{
			PackagesBySubdir: map[string]ensure.PackageSlice{
				"": {
					{PackageTemplate: "infra/3pp/tools/cpython/${platform}", UnresolvedVersion: version},
				},
			},
		},
	}
}

func CPython3FromCIPD(version string) generators.Generator {
	return &generators.CIPDExport{
		Name: "cpython",
		Ensure: ensure.File{
			PackagesBySubdir: map[string]ensure.PackageSlice{
				"": {
					{PackageTemplate: "infra/3pp/tools/cpython3/${platform}", UnresolvedVersion: version},
				},
			},
		},
	}
}

func VirtualenvFromCIPD(version string) generators.Generator {
	return &generators.CIPDExport{
		Name: "virtualenv",
		Ensure: ensure.File{
			PackagesBySubdir: map[string]ensure.PackageSlice{
				"": {
					{PackageTemplate: "infra/3pp/tools/virtualenv", UnresolvedVersion: version},
				},
			},
		},
	}
}

//go:embed bootstrap.py pep425tags.py uv_bootstrap.py
var bootstrapEmbed embed.FS
var bootstrapGen = generators.InitEmbeddedFS("bootstrap", bootstrapEmbed)

func (e *Environment) Pep425Tags() generators.Generator {
	// Generate an empty virtual environment to probe the pep425tags
	empty := &workflow.Generator{
		Name: "python_venv",
		Args: []string{
			common.Python("{{.cpython}}", e.Executable),
			filepath.Join("{{.bootstrap}}", "bootstrap.py"),
		},
		Dependencies: []generators.Dependency{
			{Type: generators.DepsHostTarget, Generator: e.CPython, Runtime: true},
			{Type: generators.DepsHostTarget, Generator: e.Virtualenv},
			{Type: generators.DepsHostTarget, Generator: bootstrapGen},
		},
	}
	return &workflow.Generator{
		Name: "python_pep425tags",
		Args: []string{
			common.PythonVENV("{{.python_venv}}", e.Executable),
			filepath.Join("{{.bootstrap}}", "pep425tags.py"),
		},
		Dependencies: []generators.Dependency{
			{Type: generators.DepsHostTarget, Generator: empty},
			{Type: generators.DepsHostTarget, Generator: bootstrapGen},
		},
	}
}

func (e *Environment) WithWheels(wheels generators.Generator) generators.Generator {
	env := environ.New(nil)
	if v := os.Getenv(common.EnvVpythonArUrl); v != "" {
		env.Set(common.EnvVpythonArUrl, v)
	}
	cipdPath := "cipd"
	if path, err := exec.LookPath("cipd"); err == nil {
		cipdPath = path
	}
	env.Set(common.EnvVpythonCipdPath, cipdPath)
	return &workflow.Generator{
		Name: "python_venv",
		Args: []string{
			common.Python("{{.cpython}}", e.Executable),
			"-BsE", // -B: no .pyc files; -s: no user site-packages; -E: ignore environment overrides
			filepath.Join("{{.bootstrap}}", "bootstrap.py"),
		},
		Env: env,
		Dependencies: []generators.Dependency{
			{Type: generators.DepsHostTarget, Generator: e.CPython, Runtime: true},
			{Type: generators.DepsHostTarget, Generator: e.Virtualenv},
			{Type: generators.DepsHostTarget, Generator: wheels},
			{Type: generators.DepsHostTarget, Generator: bootstrapGen},
		},
	}
}

func CPythonFromPath(dir, cipdName string) (generators.Generator, error) {
	cpythonDir := dir
	if !filepath.IsAbs(dir) {
		execDir, err := FindExecutableDir(nil)
		if err != nil {
			return nil, err
		}
		cpythonDir = filepath.Join(execDir, dir)
	}

	version := dir // Default to folder name for offline local developer test packaging.
	if v, err := os.Open(filepath.Join(cpythonDir, ".versions", fmt.Sprintf("%s.cipd_version", cipdName))); err == nil {
		defer v.Close()
		if fb, readErr := io.ReadAll(v); readErr == nil {
			version = string(fb)
		}
	}
	return &generators.ImportTargets{
		Name: "cpython",
		Targets: map[string]generators.ImportTarget{
			".": {Source: cpythonDir, Version: string(version), Mode: fs.ModeDir, FollowSymlinks: true},
		},
	}, nil
}

// FindExecutableDir returns the absolute directory path of the current running executable,
// resolving any symlinks under POSIX platforms (non-Windows) for path safety.
// It supports dependency injection by accepting a custom executable lookup callback,
// falling back to the standard os.Executable if nil.
func FindExecutableDir(lookupExe func() (string, error)) (string, error) {
	if lookupExe == nil {
		lookupExe = os.Executable
	}
	path, err := lookupExe()
	if err != nil {
		return "", errors.Fmt("failed to get current executable path: %w", err)
	}
	if runtime.GOOS != "windows" {
		if path, err = filepath.EvalSymlinks(path); err != nil {
			return "", errors.Fmt("failed to resolve executable symlinks: %w", err)
		}
	}
	return filepath.Dir(path), nil
}

// UVFromPath imports the pre-packaged uv directory next to the binary as a standard cipkg Generator.
func UVFromPath(dir string) (generators.Generator, error) {
	if !filepath.IsAbs(dir) {
		execDir, err := FindExecutableDir(nil)
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(execDir, dir)
	}
	versionFile := filepath.Join(dir, ".versions", "uv.cipd_version")
	version := filepath.Base(dir) // Default to folder name for offline local developer test packaging.
	if fb, err := os.ReadFile(versionFile); err == nil {
		version = strings.TrimSpace(string(fb))
	}

	return &generators.ImportTargets{
		Name: "uv",
		Targets: map[string]generators.ImportTarget{
			".": {Source: dir, Version: version, Mode: fs.ModeDir, FollowSymlinks: true},
		},
	}, nil
}

// SpecRequirementsGenerator writes ProjectSpec dependencies into a requirements.txt file.
type SpecRequirementsGenerator struct {
	Spec *standard.ProjectSpec
}

// Generate returns the copy Action for the requirements file.
func (s *SpecRequirementsGenerator) Generate(_ context.Context, plats generators.Platforms) (*core.Action, error) {
	var sb strings.Builder
	for _, req := range s.Spec.Dependencies {
		sb.WriteString(req)
		sb.WriteByte('\n')
	}
	return &core.Action{
		Name: "vpython_requirements",
		Spec: &core.Action_Copy{
			Copy: &core.ActionFilesCopy{
				Files: map[string]*core.ActionFilesCopy_Source{
					"requirements.txt": {
						Content: &core.ActionFilesCopy_Source_Raw{
							Raw: []byte(sb.String()),
						},
						Mode: 0o444,
					},
				},
			},
		},
	}, nil
}

// WithUV returns a workflow.Generator for the UV virtualenv.
func (e *Environment) WithUV(uv generators.Generator, spec *standard.ProjectSpec) generators.Generator {
	env := environ.New(nil)
	if v := os.Getenv(common.EnvVpythonArUrl); v != "" {
		env.Set(common.EnvVpythonArUrl, v)
	}

	reqGen := &SpecRequirementsGenerator{Spec: spec}

	uvBin := filepath.Join("{{.uv}}", "uv")
	if runtime.GOOS == "windows" {
		uvBin += ".exe"
	}

	return &workflow.Generator{
		Name: "uv_venv",
		Args: []string{
			common.Python("{{.cpython}}", e.Executable),
			"-BsE", // -B: no .pyc files; -s: no user site-packages; -E: ignore environment overrides
			filepath.Join("{{.bootstrap}}", "uv_bootstrap.py"),
			"--uv-bin", uvBin,
			"--python-bin", common.Python("{{.cpython}}", e.Executable),
			"--req-file", filepath.Join("{{.vpython_requirements}}", "requirements.txt"),
		},
		Env: env,
		Dependencies: []generators.Dependency{
			{Type: generators.DepsHostTarget, Generator: e.CPython, Runtime: true},
			{Type: generators.DepsHostTarget, Generator: uv},
			{Type: generators.DepsHostTarget, Generator: reqGen},
			{Type: generators.DepsHostTarget, Generator: bootstrapGen},
		},
	}
}
