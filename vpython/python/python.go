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
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/base/workflow"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/vpython/common"
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

//go:embed bootstrap.py pep425tags.py
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
	return &workflow.Generator{
		Name: "python_venv",
		Args: []string{
			common.Python("{{.cpython}}", e.Executable),
			"-BssE",
			filepath.Join("{{.bootstrap}}", "bootstrap.py"),
		},
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
		path, err := os.Executable()
		if err != nil {
			return nil, errors.Annotate(err, "failed to get executable").Err()
		}
		if runtime.GOOS != "windows" {
			if path, err = filepath.EvalSymlinks(path); err != nil {
				return nil, errors.Annotate(err, "failed to eval symlink to executable").Err()
			}
		}
		cpythonDir = filepath.Join(filepath.Dir(path), dir)
	}

	v, err := os.Open(filepath.Join(cpythonDir, ".versions", fmt.Sprintf("%s.cipd_version", cipdName)))
	if err != nil {
		return nil, errors.Annotate(err, "Bundled Python %s not found. Use VPYTHON_BYPASS if prebuilt cpython not available on this platform", dir).Err()
	}
	defer v.Close()
	version, err := io.ReadAll(v)
	if err != nil {
		return nil, errors.Annotate(err, "failed to read version file").Err()
	}
	return &generators.ImportTargets{
		Name: "cpython",
		Targets: map[string]generators.ImportTarget{
			".": {Source: cpythonDir, Version: string(version), Mode: fs.ModeDir, FollowSymlinks: true},
		},
	}, nil
}
