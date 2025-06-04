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

package generators

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"

	"go.chromium.org/luci/cipkg/core"
)

// CIPDExport is used for downloading CIPD packages. It behaves similar to
// `cipd export` for the provided ensure file and use ${out} as the cipd
// root path.
// TODO(crbug/1323147): Replace direct call cipd binary with cipd sdk when it's
// available.
type CIPDExport struct {
	Name     string
	Metadata *core.Action_Metadata

	Ensure   ensure.File
	Expander template.Expander

	ConfigFile          string
	CacheDir            string
	HTTPUserAgentPrefix string
	MaxThreads          int
	ParallelDownloads   int
	ServiceURL          string
}

func (c *CIPDExport) Generate(ctx context.Context, plats Platforms) (*core.Action, error) {
	// Expand template based on ctx.Platform.Host before pass it to cipd client
	// for cross-compile
	expander := c.Expander
	if expander == nil {
		expander = template.Platform{
			OS:   cipdOS(plats.Host.OS()),
			Arch: cipdArch(plats.Host.Arch()),
		}.Expander()
	}

	ef, err := expandEnsureFile(&c.Ensure, expander)
	if err != nil {
		return nil, fmt.Errorf("failed to expand ensure file: %v: %w", c.Ensure, err)
	}

	var w strings.Builder
	if err := ef.Serialize(&w); err != nil {
		return nil, fmt.Errorf("failed to encode ensure file: %v: %w", ef, err)
	}

	env := environ.New(nil)
	addEnv := func(k string, v any) {
		if vv := reflect.ValueOf(v); !vv.IsValid() || vv.IsZero() {
			return
		}
		env.Set(k, fmt.Sprintf("%v", v))
	}

	addEnv(cipd.EnvConfigFile, c.ConfigFile)
	addEnv(cipd.EnvCacheDir, c.CacheDir)
	addEnv(cipd.EnvHTTPUserAgentPrefix, c.HTTPUserAgentPrefix)
	addEnv(cipd.EnvMaxThreads, c.MaxThreads)
	addEnv(cipd.EnvParallelDownloads, c.ParallelDownloads)
	addEnv(cipd.EnvCIPDServiceURL, c.ServiceURL)

	return &core.Action{
		Name:     c.Name,
		Metadata: c.Metadata,
		Spec: &core.Action_Cipd{
			Cipd: &core.ActionCIPDExport{
				EnsureFile: w.String(),
				Env:        env.Sorted(),
			},
		},
	}, nil
}

func expandEnsureFile(ef *ensure.File, expander template.Expander) (*ensure.File, error) {
	ef = ef.Clone()

	for dir, slice := range ef.PackagesBySubdir {
		var s ensure.PackageSlice
		for _, p := range slice {
			pkg, err := p.Expand(expander)
			switch err {
			case template.ErrSkipTemplate:
				continue
			case nil:
			default:
				return nil, errors.Fmt("expanding %#v: %w", pkg, err)
			}
			p.PackageTemplate = pkg
			s = append(s, p)
		}
		ef.PackagesBySubdir[dir] = s
	}
	return ef, nil
}

// TODO(fancl): move these to go.chromium.org/luci/cipd as utilities.
func cipdOS(os string) string {
	if os == "darwin" {
		return "mac"
	}
	return os
}

func cipdArch(arch string) string {
	if arch == "arm" {
		return "armv6l"
	}
	return arch
}
