// Copyright 2019 The LUCI Authors.
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

package lucicfg

import (
	"flag"
	"fmt"

	luciflag "go.chromium.org/luci/common/flag"

	"go.starlark.net/starlark"
)

// Meta contains configuration for the configuration generator itself.
//
// It influences how generator produces output configs. It is settable through
// core.meta(...) statement on the Starlark side or through command line flags.
// Command line flags override what's in the Starlark.
//
// See @stdlib//internal/meta.star for full meaning of fields.
type Meta struct {
	ConfigServiceHost string   // LUCI config host name
	ConfigSet         string   // e.g. "project/<name>"
	FailOnWarnings    bool     // true to treat validation warnings as errors
	ConfigDir         string   // output directory
	TrackedFiles      []string // e.g. ["*.cfg", "!*-dev.cfg"]
}

// AddValidationFlags registers command line flags related to config validation.
func (m *Meta) AddValidationFlags(fs *flag.FlagSet) {
	fs.StringVar(&m.ConfigServiceHost, "config-service-host", m.ConfigServiceHost, "Hostname of a LUCI config service to use for validation.")
	fs.StringVar(&m.ConfigSet, "config-set", m.ConfigSet, "Name of the config set to validate against.")
	fs.BoolVar(&m.FailOnWarnings, "fail-on-warnings", m.FailOnWarnings, "Treat validation warnings as errors.")
}

// AddOutputFlags registers command line flags that specify where to put
// generated files.
func (m *Meta) AddOutputFlags(fs *flag.FlagSet) {
	fs.StringVar(&m.ConfigDir, "config-dir", m.ConfigDir, "A directory to place generated configs into.")
	fs.Var(luciflag.StringSlice(&m.TrackedFiles), "tracked-files", "Globs for files considered generated, see core.meta(...) doc for more info.")
}

// setter returns a field setter given its Starlark name.
func (m *Meta) setter(k string) func(v starlark.Value) error {
	switch k {
	case "config_service_host":
		return strSetter(&m.ConfigServiceHost)
	case "config_set":
		return strSetter(&m.ConfigSet)
	case "fail_on_warnings":
		return boolSetter(&m.FailOnWarnings)
	case "config_dir":
		return strSetter(&m.ConfigDir)
	case "tracked_files":
		return strListSetter(&m.TrackedFiles)
	default:
		return nil
	}
}

func strSetter(ptr *string) func(starlark.Value) error {
	return func(v starlark.Value) error {
		if str, ok := v.(starlark.String); ok {
			*ptr = str.GoString()
			return nil
		}
		return fmt.Errorf("set_meta: got %s, expecting string", v.Type())
	}
}

func boolSetter(ptr *bool) func(starlark.Value) error {
	return func(v starlark.Value) error {
		if b, ok := v.(starlark.Bool); ok {
			*ptr = bool(b)
			return nil
		}
		return fmt.Errorf("set_meta: got %s, expecting bool", v.Type())
	}
}

func strListSetter(ptr *[]string) func(starlark.Value) error {
	return func(v starlark.Value) error {
		if iterable, ok := v.(starlark.Iterable); ok {
			iter := iterable.Iterate()
			defer iter.Done()

			*ptr = nil

			for x := starlark.Value(nil); iter.Next(&x); {
				val := ""
				if err := strSetter(&val)(x); err != nil {
					return err
				}
				*ptr = append(*ptr, val)
			}

			return nil
		}
		return fmt.Errorf("set_meta: got %s, expecting an iterable", v.Type())
	}
}

func init() {
	// set_meta(k, v) sets the value of the corresponding field in Meta.
	//
	// Does no validation. It happens either in the starlark code, or after
	// starlark finishes running when Meta is interpreted.
	declNative("set_meta", func(call nativeCall) (starlark.Value, error) {
		var k starlark.String
		var v starlark.Value
		if err := call.unpack(2, &k, &v); err != nil {
			return nil, err
		}
		if setter := call.State.Meta.setter(k.GoString()); setter != nil {
			return starlark.None, setter(v)
		}
		return nil, fmt.Errorf("set_meta: no such meta key %s", k)
	})
}
