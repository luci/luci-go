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
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/filesystem"

	vpythonAPI "go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/python"
	"go.chromium.org/luci/vpython/spec"
	"go.chromium.org/luci/vpython/standard"
)

type FlowType int

const (
	FlowLegacy FlowType = iota
	FlowUV
)

type DiscoveredFlow struct {
	Flow            FlowType
	StandardSpec    *standard.ProjectSpec
	VpythonSpec     *vpythonAPI.Spec
	ProjectRoot     string
	FromVpythonTOML bool
}

// ResolveFlow climbs parent trees to discover standard (PEP 723, vpython.toml) or legacy specs.
func ResolveFlow(ctx context.Context, target python.Target, specPath, defaultSpecPath, defaultSpecPattern, workDir string) (*DiscoveredFlow, error) {
	// Extract script path for companion checks.
	var scriptPath string
	isModule := false
	if script, ok := target.(python.ScriptTarget); ok && script.Path != "-" {
		scriptPath = script.Path
		if err := filesystem.AbsPath(&scriptPath); err != nil {
			return nil, errors.Fmt("failed to get absolute path of target %q: %w", scriptPath, err)
		}
		if st, err := os.Stat(scriptPath); err == nil {
			isModule = st.IsDir()
		}
	}

	// Handle explicit spec path (with potential redirection for missing files).
	if specPath != "" {
		if err := filesystem.AbsPath(&specPath); err != nil {
			return nil, errors.Fmt("failed to get absolute path of spec %q: %w", specPath, err)
		}

		if filepath.Base(specPath) == "vpython.toml" || strings.EqualFold(filepath.Ext(specPath), ".toml") {
			projectSpec, err := standard.ParseVpythonTOML(specPath)
			if err != nil {
				return nil, errors.Fmt("explicit TOML spec %q is invalid: %w", specPath, err)
			}
			return &DiscoveredFlow{
				Flow:            FlowUV,
				StandardSpec:    projectSpec,
				ProjectRoot:     filepath.Dir(specPath),
				FromVpythonTOML: true,
			}, nil
		}
		// Try parsing as legacy spec.
		var sp vpythonAPI.Spec
		if err := spec.Load(specPath, &sp); err != nil {
			return nil, err
		}
		return &DiscoveredFlow{
			Flow:        FlowLegacy,
			VpythonSpec: sp.Clone(),
		}, nil
	}

	// Parse inline specs from the script (PEP 723 or legacy comments).
	if scriptPath != "" && !isModule {
		// Parse PEP 723 shebang.
		projectSpec, err := standard.ParseScriptMetadata(scriptPath)
		if err != nil {
			return nil, err
		}
		if projectSpec != nil {
			return &DiscoveredFlow{
				Flow:         FlowUV,
				StandardSpec: projectSpec,
				ProjectRoot:  filepath.Dir(scriptPath),
			}, nil
		}

		// Parse inline legacy comments.
		loader := &spec.Loader{
			InlineBeginGuard: spec.DefaultInlineBeginGuard,
			InlineEndGuard:   spec.DefaultInlineEndGuard,
		}
		legacySpec, err := loader.ParseFrom(scriptPath)
		if err == nil && legacySpec != nil {
			return &DiscoveredFlow{
				Flow:        FlowLegacy,
				VpythonSpec: legacySpec,
			}, nil
		}
	}

	// Check adjacent companion spec and determine start directory for parent climbing.
	specPattern := defaultSpecPattern
	if specPattern == "" {
		specPattern = ".vpython3"
	}

	var startDir string
	if scriptPath != "" {
		loader := &spec.Loader{
			PartnerSuffix: specPattern,
		}
		partnerSpecPath, err := loader.FindForScript(scriptPath, isModule)
		if err == nil && partnerSpecPath == "" && runtime.GOOS != "windows" {
			if realPath, err := filepath.EvalSymlinks(scriptPath); err == nil {
				partnerSpecPath, _ = loader.FindForScript(realPath, isModule)
			}
		}
		if partnerSpecPath != "" {
			var sp vpythonAPI.Spec
			if err := spec.Load(partnerSpecPath, &sp); err == nil {
				return &DiscoveredFlow{
					Flow:        FlowLegacy,
					VpythonSpec: sp.Clone(),
				}, nil
			}
		}

		if isModule {
			startDir = scriptPath
		} else {
			startDir = filepath.Dir(scriptPath)
		}
	} else {
		startDir = workDir
		if startDir == "" {
			wd, err := os.Getwd()
			if err != nil {
				return nil, errors.Fmt("failed to get working directory: %w", err)
			}
			startDir = wd
		}
		if err := filesystem.AbsPath(&startDir); err != nil {
			return nil, errors.Fmt("failed to get absolute working directory: %w", err)
		}
	}

	// Climb parent directory trees.
	loader := &spec.Loader{
		CommonSpecNames:          []string{"vpython.toml", specPattern},
		CommonFilesystemBarriers: []string{".gclient"},
	}

	foundSpecPath, err := loader.FindCommonWalkingFrom(startDir)
	if err != nil {
		return nil, err
	}
	if foundSpecPath == "" {
		return loadDefaultFlow(defaultSpecPath)
	}

	if filepath.Base(foundSpecPath) == "vpython.toml" {
		projectSpec, err := standard.ParseVpythonTOML(foundSpecPath)
		if err != nil {
			return nil, err
		}
		if projectSpec == nil {
			return nil, errors.Fmt("vpython.toml at %s is empty or missing standard spec blocks", foundSpecPath)
		}
		return &DiscoveredFlow{
			Flow:            FlowUV,
			StandardSpec:    projectSpec,
			ProjectRoot:     filepath.Dir(foundSpecPath),
			FromVpythonTOML: true,
		}, nil
	}

	// Found legacy common spec.
	var sp vpythonAPI.Spec
	if err := spec.Load(foundSpecPath, &sp); err != nil {
		return nil, err
	}
	return &DiscoveredFlow{
		Flow:        FlowLegacy,
		VpythonSpec: sp.Clone(),
	}, nil
}

func loadDefaultFlow(defaultSpecPath string) (*DiscoveredFlow, error) {
	sp := &vpythonAPI.Spec{}
	if defaultSpecPath != "" {
		if err := spec.Load(defaultSpecPath, sp); err != nil {
			return nil, errors.Fmt("failed to load default spec: %#v: %w", defaultSpecPath, err)
		}
	}
	return &DiscoveredFlow{
		Flow:        FlowLegacy,
		VpythonSpec: sp,
	}, nil
}
