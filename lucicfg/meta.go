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
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	luciflag "go.chromium.org/luci/common/flag"
	"go.chromium.org/luci/common/logging"

	"go.starlark.net/starlark"
)

// Meta contains configuration for the configuration generator itself.
//
// It influences how generator produces output configs. It is settable through
// meta.config(...) statements on the Starlark side or through command line
// flags. Command line flags override what was set via meta.config(...).
//
// See @stdlib//internal/meta.star for full meaning of fields.
type Meta struct {
	ConfigServiceHost string   `json:"config_service_host"` // LUCI config host name
	ConfigSet         string   `json:"config_set"`          // e.g. "project/<name>"
	ConfigDir         string   `json:"config_dir"`          // output directory to place generated files or '-' for stdout
	TrackedFiles      []string `json:"tracked_files"`       // e.g. ["*.cfg", "!*-dev.cfg"]
	FailOnWarnings    bool     `json:"fail_on_warnings"`    // true to treat validation warnings as errors

	// FlagSet passed to AddFlags and AddOutputFlags.
	fs *flag.FlagSet
	// Pointers to fields that were touched. Used when merging two Metas together.
	touched map[interface{}]struct{}
	// True if detectTouchedFlags() was already called.
	detectedTouchedFlags bool
}

// Log logs the values of the meta parameters to Debug logger.
func (m *Meta) Log(ctx context.Context) {
	logging.Debugf(ctx, "Meta config:")
	logging.Debugf(ctx, "  config_service_host = %q", m.ConfigServiceHost)
	logging.Debugf(ctx, "  config_set = %q", m.ConfigSet)
	logging.Debugf(ctx, "  config_dir = %q", m.ConfigDir)
	logging.Debugf(ctx, "  tracked_files = %v", m.TrackedFiles)
	logging.Debugf(ctx, "  fail_on_warnings = %v", m.FailOnWarnings)
}

// RebaseConfigDir changes ConfigDir, if it is set, to be absolute by appending
// it to the given root.
//
// Doesn't touch "-", which indicates "stdout".
func (m *Meta) RebaseConfigDir(root string) {
	if m.ConfigDir != "" && m.ConfigDir != "-" {
		m.ConfigDir = filepath.Join(root, filepath.FromSlash(m.ConfigDir))
	}
}

// AddFlags registers command line flags that correspond to Meta fields.
func (m *Meta) AddFlags(fs *flag.FlagSet) {
	m.fs = fs
	fs.StringVar(&m.ConfigServiceHost, "config-service-host", m.ConfigServiceHost, "Hostname of a LUCI config service to use for validation.")
	fs.StringVar(&m.ConfigSet, "config-set", m.ConfigSet, "Name of the config set to validate against.")
	fs.StringVar(&m.ConfigDir, "config-dir", m.ConfigDir,
		`A directory to place generated configs into (relative to cwd if given as a
flag otherwise relative to the main script). If '-', generated configs are just
printed to stdout in a format useful for debugging.`)
	fs.Var(luciflag.CommaList(&m.TrackedFiles), "tracked-files", "Globs for files considered generated, see meta.config(...) doc for more info.")
	fs.BoolVar(&m.FailOnWarnings, "fail-on-warnings", m.FailOnWarnings, "Treat validation warnings as errors.")
}

// detectTouchedFlags is called after flags are parsed to figure out what flags
// were explicitly set and what were left at their defaults.
//
// It updates Meta with information about touched flags which is later used
// by PopulateFromTouchedIn and WasTouched functions.
func (m *Meta) detectTouchedFlags() {
	switch {
	case m.fs == nil:
		return // not using CLI flags at all
	case m.detectedTouchedFlags:
		return // already did this
	case !m.fs.Parsed():
		panic("detectTouchedFlags should be called after flags are parsed")
	}
	m.detectedTouchedFlags = true

	fields := m.fieldsMap()

	m.fs.Visit(func(f *flag.Flag) {
		if ptr := fields[strings.Replace(f.Name, "-", "_", -1)]; ptr != nil {
			m.touch(ptr)
		}
	})
}

// PopulateFromTouchedIn takes all touched values in `t` and copies them to
// `m`, overriding what's in `m`.
func (m *Meta) PopulateFromTouchedIn(t *Meta) {
	t.detectTouchedFlags()
	left := m.fieldsMap()
	right := t.fieldsMap()
	for k, l := range left {
		r := right[k]
		if _, yes := t.touched[r]; yes {
			// Do *l = *r.
			switch l.(type) {
			case *string:
				*(l.(*string)) = *(r.(*string))
			case *bool:
				*(l.(*bool)) = *(r.(*bool))
			case *[]string:
				*(l.(*[]string)) = append([]string(nil), *(r.(*[]string))...)
			default:
				panic("impossible")
			}
		}
	}
}

// WasTouched returns true if the field (given by its Starlark snake_case name)
// was explicitly set via CLI flags or via meta.config(...) in Starlark.
//
// Panics if the field is unrecognized.
func (m *Meta) WasTouched(field string) bool {
	m.detectTouchedFlags()
	ptr, ok := m.fieldsMap()[field]
	if !ok {
		panic(fmt.Sprintf("no such meta field %s", field))
	}
	_, yes := m.touched[ptr]
	return yes
}

////////////////////////////////////////////////////////////////////////////////

// touch takes a pointer to some Meta field and marks it as explicitly set.
func (m *Meta) touch(ptr interface{}) {
	if m.touched == nil {
		m.touched = make(map[interface{}]struct{}, 1)
	}
	m.touched[ptr] = struct{}{}
}

// fieldsMap returns a mapping from a snake_case name of a field to a pointer to
// this field inside 'm'.
//
// This is used by both Starlark accessors and for processing CLI flags.
func (m *Meta) fieldsMap() map[string]interface{} {
	return map[string]interface{}{
		"config_service_host": &m.ConfigServiceHost,
		"config_set":          &m.ConfigSet,
		"config_dir":          &m.ConfigDir,
		"tracked_files":       &m.TrackedFiles,
		"fail_on_warnings":    &m.FailOnWarnings,
	}
}

// setField sets the field k to v.
//
// If untouched is true, will do it only if k wasn't modified before. Silently
// does nothing if it was.
func (m *Meta) setField(k string, v starlark.Value, untouched bool) (err error) {
	ptr := m.fieldsMap()[k]
	if ptr == nil {
		return fmt.Errorf("set_meta: no such meta key %q", k)
	}

	if untouched && m.WasTouched(k) {
		return nil
	}

	// On success, mark the field as modified.
	defer func() {
		if err == nil {
			m.touch(ptr)
		}
	}()

	switch ptr := ptr.(type) {
	case *string:
		if str, ok := v.(starlark.String); ok {
			*ptr = str.GoString()
			return nil
		}
		return fmt.Errorf("set_meta: got %s, expecting string", v.Type())

	case *bool:
		if b, ok := v.(starlark.Bool); ok {
			*ptr = bool(b)
			return nil
		}
		return fmt.Errorf("set_meta: got %s, expecting bool", v.Type())

	case *[]string:
		if iterable, ok := v.(starlark.Iterable); ok {
			iter := iterable.Iterate()
			defer iter.Done()

			var vals []string
			for x := starlark.Value(nil); iter.Next(&x); {
				if str, ok := x.(starlark.String); ok {
					vals = append(vals, str.GoString())
				} else {
					return fmt.Errorf("set_meta: got %s, expecting string", x.Type())
				}
			}

			*ptr = vals
			return nil
		}
		return fmt.Errorf("set_meta: got %s, expecting an iterable", v.Type())

	default:
		panic("impossible")
	}
}

func init() {
	// set_meta(k, v) sets the value of the corresponding field in Meta.
	declNative("set_meta", func(call nativeCall) (starlark.Value, error) {
		var k starlark.String
		var v starlark.Value
		if err := call.unpack(2, &k, &v); err != nil {
			return nil, err
		}
		return starlark.None, call.State.Meta.setField(k.GoString(), v, false)
	})

	// set_meta_default(k, v) sets the value of the corresponding field in Meta,
	// but only if it wasn't set before.
	//
	// Unlike set_meta, which is almost directly exposed through meta.config(...),
	// set_meta_default is currently a private API, used only to set the default
	// config set name based on core.project(name=...). It is expected that no
	// one will really need to use set_meta_default, so it's not worth exporting
	// and documenting it.
	declNative("set_meta_default", func(call nativeCall) (starlark.Value, error) {
		var k starlark.String
		var v starlark.Value
		if err := call.unpack(2, &k, &v); err != nil {
			return nil, err
		}
		return starlark.None, call.State.Meta.setField(k.GoString(), v, true)
	})
}
