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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

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
	"go.chromium.org/luci/vpython/common"
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
	if ar := os.Getenv(common.EnvVpythonArUrl); ar != "" {
		env.Set(common.EnvVpythonArUrl, ar)
	}
	if common.VpythonCacheSalt != "" {
		env.Set(common.EnvVpythonCacheSalt, common.VpythonCacheSalt)
	}
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

	// Generate requirements.txt from the spec.
	if err := writeRequirementsFromSpec(filepath.Join(out, "requirements.txt"), s, tags); err != nil {
		return errors.Fmt("failed to write requirements.txt: %w", err)
	}

	// Write target_arch.txt if the resolved tag is x86_64 (amd64 in template terms).
	// Done as there is no direct communication between python and go.
	if t := pep425TagSelector(tags); t != nil {
		if p := PlatformForPEP425Tag(t); p.Arch == "amd64" {
			if err := os.WriteFile(filepath.Join(out, "target_arch.txt"), []byte("x86_64"), 0644); err != nil {
				return err
			}
		}
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

var nameNormalizationRE = regexp.MustCompile(`[-_.]+`)

// CIPD package names for wheels are usually:
// infra/python/wheels/<name>/${vpython_platform}
// or similar variants like:
// infra/python/wheels/<name>-py2_py3
func pipNameFromPackageName(name string) string {
	res := name
	const prefix = "infra/python/wheels/"
	// Drop common CIPD prefix and get the package name that comes
	// right after it:
	// infra/python/wheels/six-py3/${vpython_platform}
	if after, ok := strings.CutPrefix(name, prefix); ok {
		res = after
		parts := strings.Split(res, "/")
		res = parts[0]
	} else {
		// In an off chance that the prefix is not what we expect,
		// get the package name that is either the last / block or
		// just before the ${vpython_platform}.
		// For instance infra/tools/python/six-py3/${vpython_platform}
		parts := strings.Split(name, "/")
		for i := len(parts) - 1; i >= 0; i-- {
			p := parts[i]
			if !strings.Contains(p, "${") && p != "" {
				res = p
				break
			}
		}
	}

	// Strip common suffixes for universal wheels.
	for _, suffix := range []string{"-py2_py3", "_py2_py3", "-py3", "_py3", "-py2", "_py2"} {
		if strings.HasSuffix(res, suffix) {
			res = res[:len(res)-len(suffix)]
			break
		}
	}

	// PEP 503 normalization: lowercase and replace runs of [._-] with a single hyphen.
	return nameNormalizationRE.ReplaceAllString(strings.ToLower(res), "-")
}

func pipVersionFromPackageVersion(version string) string {
	v := version
	if strings.HasPrefix(v, "version:2@") {
		v = v[10:]
	} else if strings.HasPrefix(v, "version:") {
		v = v[8:]
	}

	// Check if version starts with a digit (or v followed by digit)
	if len(v) == 0 {
		return v
	}
	if !unicode.IsDigit(rune(v[0])) && !(v[0] == 'v' && len(v) > 1 && unicode.IsDigit(rune(v[1]))) {
		return v // Not a standard version, skip normalization
	}

	// Split by ., - and +
	segments := strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-' || r == '+'
	})

	for _, seg := range segments {
		if len(seg) == 0 {
			continue
		}
		allDigits := true
		for _, r := range seg {
			if !unicode.IsDigit(r) {
				allDigits = false
				break
			}
		}
		if !allDigits {
			// Check if it's a valid pre-release segment
			isPre := false
			for _, pre := range []string{"a", "b", "rc", "alpha", "beta", "pre", "preview", "c", "post", "rev", "r", "dev"} {
				if strings.HasPrefix(strings.ToLower(seg), pre) {
					// Check if the rest of the segment is numeric
					rest := seg[len(pre):]
					isNumeric := true
					for _, r := range rest {
						if !unicode.IsDigit(r) {
							isNumeric = false
							break
						}
					}
					if isNumeric {
						isPre = true
						break
					}
				}
			}
			if !isPre {
				// Found a non-pre-release segment starting with a letter.
				// This is the start of the local version.
				// We want to replace the separator BEFORE this segment with +.
				idx := strings.Index(v, seg)
				if idx > 0 {
					sep := v[idx-1]
					if sep == '.' || sep == '-' {
						// Replace the separator with +
						return v[:idx-1] + "+" + v[idx:]
					}
				}
			}
		}
	}

	return v
}

func writeRequirementsFromSpec(path string, s *vpython.Spec, tags []*vpython.PEP425Tag) (err error) {
	// Use ensureFileFromVPythonSpec to filter and expand template names.
	ef, err := ensureFileFromVPythonSpec(s, tags)
	if err != nil {
		return err
	}

	fd, err := os.Create(path)
	if err != nil {
		return errors.Fmt("failed to create requirements file: %w", err)
	}
	defer func() {
		if closeErr := fd.Close(); closeErr != nil && err == nil {
			err = errors.Fmt("failed to close requirements file: %w", closeErr)
		}
	}()

	seen := make(map[string]struct{})
	for _, pkg := range ef.PackagesBySubdir["wheels"] {
		if pkg.PackageTemplate == "" {
			continue // package was skipped
		}
		name := pipNameFromPackageName(pkg.PackageTemplate)
		version := pipVersionFromPackageVersion(pkg.UnresolvedVersion)
		line := fmt.Sprintf("%s==%s", name, version)
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		if _, err := fmt.Fprintf(fd, "%s\n", line); err != nil {
			return errors.Fmt("failed to write requirement: %w", err)
		}
	}
	return nil
}
