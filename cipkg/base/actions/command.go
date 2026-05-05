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

package actions

import (
	"fmt"
	"path/filepath"
	"regexp"

	"go.chromium.org/luci/cipkg/core"
)

// ActionCommandTransformer is the default transformer for core.ActionCommand.
func ActionCommandTransformer(a *core.ActionCommand, deps []Package) (*core.Derivation, error) {
	drv := &core.Derivation{
		Args: a.Args,
		Env:  a.Env,
	}

	// Render templates
	if err := renderDerivation(NewRenderEnv(deps), drv); err != nil {
		return nil, err
	}

	return drv, nil
}

type RenderEnv map[string]string

// NewRenderEnv creates a mapping from package name to output path for
// rendering.
func NewRenderEnv(deps []Package) RenderEnv {
	env := make(RenderEnv)
	for _, d := range deps {
		env[d.Action.Name] = d.Handler.OutputDirectory()
	}
	return env
}

var renderRefRe = regexp.MustCompile(`{{.+?}}`)

// Render renders references in raw to the output directory.
func (e RenderEnv) Render(raw string) (ret string, err error) {
	ret = renderRefRe.ReplaceAllStringFunc(raw, func(s string) string {
		// removes "{{." and "}}" from {{.something}}
		if s[2] != '.' {
			err = fmt.Errorf("failed to render %s: use {{.key}} to reference value", raw)
			return ""
		}

		if v, ok := e[s[3:len(s)-2]]; ok {
			return v
		}
		err = fmt.Errorf("failed to render %s: unknown key %s", raw, s)
		return ""
	})

	return
}

// RenderAll renders references in raw list to the output directory.
func (e RenderEnv) RenderAll(raw []string) ([]string, error) {
	var ret []string
	for _, r := range raw {
		s, err := e.Render(r)
		if err != nil {
			return nil, err
		}
		ret = append(ret, s)
	}
	return ret, nil
}

// DepRef is a helper to convert name to reference which will be rendered by
// command Action to output directory.
func DepRef(name string, path ...string) string {
	return filepath.Join(append([]string{"{{." + name + "}}"}, path...)...)
}

func renderDerivation(e RenderEnv, drv *core.Derivation) (err error) {
	drv.Args, err = e.RenderAll(drv.Args)
	if err != nil {
		return
	}
	drv.Env, err = e.RenderAll(drv.Env)
	if err != nil {
		return err
	}
	return nil
}
