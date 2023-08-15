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
	dirs := make(map[string]string)
	for _, d := range deps {
		dirs[d.Action.Name] = d.Handler.OutputDirectory()
	}
	if err := renderDerivation(dirs, drv); err != nil {
		return nil, err
	}

	return drv, nil
}

func renderDerivation(vals map[string]string, drv *core.Derivation) (err error) {
	drv.Args, err = renderAll(drv.Args, vals)
	if err != nil {
		return
	}
	drv.Env, err = renderAll(drv.Env, vals)
	if err != nil {
		return err
	}
	return nil
}

func renderAll(raw []string, vals map[string]string) (ret []string, err error) {
	for _, r := range raw {
		re := regexp.MustCompile(`{{.+?}}`)
		rendered := re.ReplaceAllStringFunc(r, func(s string) string {
			// removes "{{." and "}}" from {{.something}}
			if s[2] != '.' {
				err = fmt.Errorf("failed to render %s: use {{.key}} to reference value", r)
				return "PANIC"
			}

			if v, ok := vals[s[3:len(s)-2]]; ok {
				return v
			}
			err = fmt.Errorf("failed to render %s: unknown key %s", r, s)
			return "PANIC"
		})
		if err != nil {
			return
		}
		ret = append(ret, rendered)
	}
	return ret, nil
}
