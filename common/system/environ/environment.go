// Copyright 2016 The LUCI Authors.
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

// Package environ is an environment variable manipulation library.
//
// It couples the system's environment, represented as a []string of KEY[=VALUE]
// strings, into a key/value map and back.
package environ

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
)

// Env contains system environment variables. It preserves each environment
// variable verbatim (even if invalid).
//
// Treat it as a map[...] (i.e. pass by value, but assume this value is
// actually a reference and the object can be mutated through it).
//
// An empty value can be read or cloned, but should not be written to. If you
// want to get an empty modifiable Env use New(nil).
type Env struct {
	// Env is a map of environment key to its "KEY=VALUE" value.
	//
	// Note that the value here is the full "KEY=VALUE", not just the VALUE
	// part. This allows us to reconstitute the original environment string slice
	// without reallocating all of its composite strings.
	env map[string]string
}

var ctxKey = "holds an Env"

// FromCtx returns a copy of the current Env in `ctx`.
//
// This is guaranteed to return a non-nil Env (i.e. it's always safe for
// assignment/manipulation).
//
// If no Env has been set with `SetInCtx`, this returns `System()`.
func FromCtx(ctx context.Context) Env {
	if cur, ok := ctx.Value(&ctxKey).(Env); ok {
		return cur.Clone()
	}
	return System()
}

// SetInCtx installs a copy of the Env into the context, which can be retrieved
// with `FromCtx`.
func (e Env) SetInCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, &ctxKey, e.Clone())
}

// System returns an Env instance instantiated with the current os.Environ
// values.
func System() Env {
	return New(os.Environ())
}

// New instantiates a new Env instance from the supplied set of environment
// KEY=VALUE strings (e.g. as returned by os.Environ()).
func New(s []string) Env {
	e := Env{env: make(map[string]string, len(s))}
	for _, entry := range s {
		e.SetEntry(entry)
	}
	return e
}

// String returns debug representation of Env.
func (e Env) String() string { return fmt.Sprintf("%v", e.Sorted()) }

// Load adds environment variables defined in a key/value map to an existing
// environment.
func (e Env) Load(m map[string]string) {
	for k, v := range m {
		e.Set(k, v)
	}
}

// Set sets the supplied environment key and value.
//
// Panics if the Env is zero-valued i.e. equals to Env{}. Use New(nil) to create
// a modifiable empty environment.
func (e Env) Set(k, v string) { e.SetEntry(Join(k, v)) }

// SetEntry sets the supplied environment to a "KEY[=VALUE]" entry.
//
// Panics if the Env is zero-valued i.e. equals to Env{}. Use New(nil) to create
// a modifiable empty environment.
func (e Env) SetEntry(entry string) {
	if e.env == nil {
		panic("cannot modify zero Env")
	}

	// "entry" must be a well-formed "key=value" entry. If it doesn't have an "=",
	// we will forcibly add one to make it well-formed.
	if strings.IndexRune(entry, '=') < 0 {
		entry += "="
	}
	k, _ := Split(entry)
	if len(k) > 0 {
		e.env[normalizeKeyCase(k)] = entry
	}
}

// Remove removes a value from the environment, returning true if the value was
// present. If the value was missing, Remove returns false.
//
// Remove is different from Set(k, "") in that Set persists the key with an
// empty value, while Remove removes it entirely.
func (e Env) Remove(k string) bool {
	k = normalizeKeyCase(k)
	if _, ok := e.env[k]; ok {
		delete(e.env, k)
		return true
	}
	return false
}

// RemoveMatch iterates over all keys and values in the environment, invoking
// the callback function, fn, for each key/value pair. If fn returns true, the
// key is removed from the environment.
func (e Env) RemoveMatch(fn func(k, v string) bool) {
	e.iterate(func(realKey, k, v string) error {
		if fn(k, v) {
			delete(e.env, realKey)
		}
		return nil
	})
}

// Update adds all key/value from other to the current environment. If there is
// a duplicate key, the value from other will overwrite the value from e.
//
// Values from other will be sorted and added in alphabetic order. This means
// that if e is case insensitive and there are multiple keys in other that
// converge on the same case insensitive key, the one that is alphabetically
// highest will be added.
//
// Panics if the Env is zero-valued i.e. equals to Env{}. Use New(nil) to create
// a modifiable empty environment.
func (e Env) Update(other Env) {
	for _, entry := range other.Sorted() {
		e.SetEntry(entry)
	}
}

// Lookup retrieves the value of the environment variable named by the key.
//
// If the variable is present in the environment the value (which may be empty)
// is returned and the boolean is true. Otherwise the returned value will be
// empty and the boolean will be false.
func (e Env) Lookup(k string) (v string, ok bool) {
	// NOTE: "v" is initially the combined "key=value" entry, and will need to be
	// split.
	if v, ok = e.env[normalizeKeyCase(k)]; ok {
		_, v = splitEntryGivenKey(k, v)
	}
	return
}

// Get retrieves the value of the environment variable named by the key.
//
// It returns the value, which will be empty if the variable is not present.
// To distinguish between an empty value and an unset value, use Lookup.
func (e Env) Get(k string) string {
	v, _ := e.Lookup(k)
	return v
}

// Sorted returns the contents of the environment, sorted by key.
func (e Env) Sorted() []string {
	var r []string
	if len(e.env) > 0 {
		r = make([]string, 0, len(e.env))
		for _, v := range e.env {
			r = append(r, v)
		}
		sort.Strings(r)
	}
	return r
}

// Map returns a map of the key/value values in the environment.
//
// This is a clone of the contents of e; manipulating this map will not change
// the values in e.
//
// If env is either nil or empty, returns nil.
func (e Env) Map() map[string]string {
	if len(e.env) == 0 {
		return nil
	}
	m := make(map[string]string, len(e.env))
	for k, entry := range e.env {
		ek, ev := splitEntryGivenKey(k, entry)
		m[ek] = ev
	}
	return m
}

// Len returns the number of environment variables defined in e.
func (e Env) Len() int { return len(e.env) }

// Clone creates a new Env instance that is identical to, but independent from,
// e.
//
// The returned clone is always safe to modify.
func (e Env) Clone() Env {
	clone := Env{env: make(map[string]string, len(e.env))}
	for k, v := range e.env {
		clone.env[k] = v
	}
	return clone
}

// Internal iteration function. Invokes cb for every entry in the environment.
// If the callback returns error, iteration stops and returns that error.
//
// It is safe to mutate the environment map during iteration.
//
// realKey is the real, normalized map key. envKey and envValue are the split
// map value.
func (e Env) iterate(cb func(realKey, envKey, envValue string) error) error {
	for k, v := range e.env {
		envKey, envValue := Split(v)
		if err := cb(k, envKey, envValue); err != nil {
			return err
		}
	}
	return nil
}

// Iter iterates through all of the key/value pairs in Env and invokes the
// supplied callback, cb, for each element.
//
// If the callback returns error, iteration will stop immediately.
func (e Env) Iter(cb func(k, v string) error) error {
	return e.iterate(func(_, k, v string) error { return cb(k, v) })
}

// Split splits the supplied environment variable value into a key/value pair.
//
// If v is of the form:
//   - KEY, returns (KEY, "")
//   - KEY=, returns (KEY, "")
//   - KEY=VALUE, returns (KEY, VALUE)
func Split(v string) (key, value string) {
	parts := strings.SplitN(v, "=", 2)
	switch len(parts) {
	case 2:
		value = parts[1]
		fallthrough

	case 1:
		key = parts[0]
	}
	return
}

// Join creates an environment variable definition for the supplied key/value.
func Join(k, v string) string {
	if v == "" {
		return k + "="
	}
	return k + "=" + v
}

// splitEntryGivenKey extracts the key and value components of the "env" map's
// combined "key=value" field, given the "key" component.
//
// This will work as long as the length of the supplied key equals the length of
// entry's key prefix, regardless of the key's contents which means that
// normalized keys can be used.
//
// It assumes that the environment variable is well-formed, one of:
// - KEY (returns "")
// - KEY= (returns "")
// - KEY=VALUE (returns "VALUE")
//
// If this is not the case, the result is undefined. However, this is an
// internal function and should never encounter a situation where the entry is
// not well-formed and prefixed by the supplied key.
func splitEntryGivenKey(key, entry string) (k, v string) {
	prefixSize := len(key) + len("=")
	switch {
	case len(entry) < prefixSize:
		return entry, ""
	case len(entry) == prefixSize:
		return entry[:len(key)], ""
	default:
		return entry[:len(key)], entry[prefixSize:]
	}
}

// normalizeKeyCase normalizes the map key by upper-casing it on Windows.
//
// Returns the key unchanged on other platforms. Mocked in tests.
var normalizeKeyCase func(string) string

func init() {
	// Note: we don't use +build flags to be able to unit test both variants on
	// a single platform just by mocking 'normalizeKeyCase' value.
	if runtime.GOOS == "windows" {
		normalizeKeyCase = strings.ToUpper
	} else {
		normalizeKeyCase = func(k string) string { return k }
	}
}
