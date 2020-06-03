// Copyright 2020 The LUCI Authors.
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

// Package vars implements a registry of ${var} placeholders.
//
// They can be used in config validation rule patterns and config set names
// and paths in place of a not-yet-known values.
package vars

import (
	"context"
	"fmt"
	"regexp"
	"sync"

	"go.chromium.org/luci/common/errors"
)

// Vars is the default set of placeholder vars used by the process.
//
// Individual packages may register vars here during init() time.
var Vars VarSet

// VarSet holds a registry ${var} -> callback that extracts it from a context.
//
// It is used by the config client to render config validation patterns like
// "services/${appid}". By using placeholder vars it is possible to register
// validation rules during process startup time before values of all vars
// are actually known.
type VarSet struct {
	l sync.RWMutex
	v map[string]func(context.Context) (string, error)
}

// Register registers a variable that can be used in templates as `${name}`.
//
// Such placeholder variable is rendered into an actual value via the given
// callback in RenderTemplate.
//
// The primary use case for this mechanism is to allow to register config
// validation rule patterns that depend on not-yet known values during init()
// time.
//
// Panics if such variable is already registered.
func (vs *VarSet) Register(name string, value func(context.Context) (string, error)) {
	vs.l.Lock()
	defer vs.l.Unlock()
	if vs.v == nil {
		vs.v = make(map[string]func(context.Context) (string, error), 1)
	}
	if vs.v[name] != nil {
		panic(fmt.Sprintf("variable %q is already registered", name))
	}
	vs.v[name] = value
}

var placeholderRe = regexp.MustCompile(`\${[^}]*}`)

// RenderTemplate replaces all `${var}` references in the string with their
// values by calling registered callbacks.
func (vs *VarSet) RenderTemplate(ctx context.Context, templ string) (string, error) {
	vs.l.RLock()
	defer vs.l.RUnlock()

	var errs errors.MultiError

	out := placeholderRe.ReplaceAllStringFunc(templ, func(match string) string {
		name := match[2 : len(match)-1] // strip ${...}

		var val string
		var err error

		if cb := vs.v[name]; cb != nil {
			val, err = cb(ctx)
		} else {
			err = fmt.Errorf("no placeholder named %q is registered", name)
		}

		if err != nil {
			errs = append(errs, err)
		}
		return val
	})

	if len(errs) != 0 {
		return "", errs
	}
	return out, nil
}
