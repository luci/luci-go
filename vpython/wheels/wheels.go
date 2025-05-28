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

// Package wheels includes implementation for installing wheels inside venv from
// vpython spec.
package wheels

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"

	"go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/spec"
)

type vpythonSpecGenerator struct {
	spec       *vpython.Spec
	pep425tags generators.Generator
}

func (g *vpythonSpecGenerator) Generate(ctx context.Context, plats generators.Platforms) (*core.Action, error) {
	p, err := g.pep425tags.Generate(ctx, plats)
	if err != nil {
		return nil, err
	}
	s, err := anypb.New(g.spec)
	if err != nil {
		return nil, err
	}
	return &core.Action{
		Name: "wheels",
		Deps: []*core.Action{p},
		Spec: &core.Action_Extension{Extension: s},
	}, nil
}

func FromSpec(spec *vpython.Spec, pep425tags generators.Generator) generators.Generator {
	return &vpythonSpecGenerator{spec: spec, pep425tags: pep425tags}
}

func MustSetTransformer(cipdCacheDir string, ap *actions.ActionProcessor) {
	v := &vpythonSpecTransformer{
		cipdCacheDir: cipdCacheDir,
	}
	actions.MustSetTransformer[*vpython.Spec](ap, v.Transform)
}

type vpythonSpecTransformer struct {
	cipdCacheDir string
}

func (v *vpythonSpecTransformer) Transform(spec *vpython.Spec, deps []actions.Package) (*core.Derivation, error) {
	drv, err := actions.ReexecDerivation(spec, true)
	if err != nil {
		return nil, err
	}
	env := environ.New(drv.Env)
	env.Set(cipd.EnvCacheDir, v.cipdCacheDir)
	for _, d := range deps {
		drv.FixedOutput += "+" + d.DerivationID
		env.Set(d.Action.Name, d.Handler.OutputDirectory())
	}
	drv.Env = env.Sorted()
	return drv, nil
}

func MustSetExecutor(reexec *actions.ReexecRegistry) {
	actions.MustSetExecutor[*vpython.Spec](reexec, actionVPythonSpecExecutor)
}

func actionVPythonSpecExecutor(ctx context.Context, s *vpython.Spec, out string) error {
	envs := environ.FromCtx(ctx)

	// Parse tags file
	var tags []*vpython.PEP425Tag
	tagsDir := envs.Get("python_pep425tags")
	raw, err := os.Open(filepath.Join(tagsDir, "pep425tags.json"))
	if err != nil {
		return err
	}
	defer raw.Close()
	if err := json.NewDecoder(raw).Decode(&tags); err != nil {
		return err
	}

	// Translates vpython spec into a CIPD ensure file.
	ef, err := ensureFileFromVPythonSpec(s, tags)
	if err != nil {
		return err
	}
	var efs strings.Builder
	if err := ef.Serialize(&efs); err != nil {
		return err
	}

	// Execute cipd export
	if err := actions.ActionCIPDExportExecutor(ctx, &core.ActionCIPDExport{
		EnsureFile: efs.String(),
		Env:        envs.Sorted(),
	}, out); err != nil {
		return err
	}

	// Generate requirements.txt
	wheels := filepath.Join(out, "wheels")
	ws, err := scanDir(wheels)
	if err != nil {
		return errors.Fmt("failed to scan wheels: %w", err)
	}
	if err := writeRequirementsFile(filepath.Join(out, "requirements.txt"), ws); err != nil {
		return errors.Fmt("failed to write requirements.txt: %w", err)
	}

	return nil
}

func ensureFileFromVPythonSpec(s *vpython.Spec, tags []*vpython.PEP425Tag) (*ensure.File, error) {
	s = proto.Clone(s).(*vpython.Spec)

	// Remove unmatched wheels from spec
	if err := spec.NormalizeSpec(s, tags); err != nil {
		return nil, err
	}

	// Get vpython template from tags
	expander := template.DefaultExpander()
	if t := pep425TagSelector(tags); t != nil {
		p := PlatformForPEP425Tag(t)
		expander = p.Expander()
		if err := addPEP425CIPDTemplateForTag(expander, t); err != nil {
			return nil, err
		}
	}

	// Construct cipd packages
	names := make(map[string]struct{})
	pslice := make(ensure.PackageSlice, len(s.Wheel))
	for i, pkg := range s.Wheel {
		name, err := expander.Expand(pkg.Name)
		if err != nil {
			if errors.Is(err, template.ErrSkipTemplate) {
				continue
			}
			return nil, errors.Fmt("expanding %v: %w", pkg, err)
		}
		if _, ok := names[name]; ok {
			return nil, errors.Fmt("duplicated package: %v", pkg)
		}
		names[name] = struct{}{}

		pslice[i] = ensure.PackageDef{
			PackageTemplate:   name,
			UnresolvedVersion: pkg.Version,
		}
	}

	return &ensure.File{
		PackagesBySubdir: map[string]ensure.PackageSlice{"wheels": pslice},
	}, nil
}
