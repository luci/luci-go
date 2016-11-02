// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package environ is a simple environment variable manipulation library. It
// couples the system's environment, representated as a []string of KEY[=VALUE]
// strings, into a key/value map and back.
package environ

import (
	"os"
	"sort"
	"strings"
)

// Env contains system environment variables. It preserves each environment
// variable verbatim (even if invalid).
type Env struct {
	// env is a map of envoironment key to its "KEY=VALUE" value.
	//
	// Note that the value here is the full "KEY=VALUE", not just the VALUE part.
	// This allows us to reconstitute the original environment string slice
	// without reallocating all of its composite strings.
	env map[string]string
}

// System returns an Env instance instantiated with the current os.Environ
// values.
func System() Env {
	return New(os.Environ())
}

// New instantiates a new Env instance from the supplied set of environment
// KEY=VALUE strings.
func New(s []string) Env {
	e := Env{}
	if len(s) > 0 {
		e.env = make(map[string]string, len(s))
		for _, v := range s {
			k, _ := Split(v)
			e.env[k] = v
		}
	}
	return e
}

// Make creaets a new Env from an environment key/value map.
func Make(v map[string]string) Env {
	e := Env{}
	if len(v) > 0 {
		e.env = make(map[string]string, len(v))
		for k, v := range v {
			e.Set(k, v)
		}
	}
	return e
}

// Get returns the environment value for the supplied key.
//
// If the value is defined, ok will be true and v will be set to its value (note
// that this can be empty if the environment has an empty value). If the value
// is not defined, ok will be false.
func (e Env) Get(k string) (v string, ok bool) {
	if v, ok = e.env[k]; ok {
		v = value(k, v)
	}
	return
}

// Set sets the supplied environment key and value.
func (e *Env) Set(k, v string) {
	if e.env == nil {
		e.env = make(map[string]string)
	}
	e.env[k] = Join(k, v)
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
func (e Env) Map() map[string]string {
	if len(e.env) == 0 {
		return nil
	}

	m := make(map[string]string, len(e.env))
	for k, v := range e.env {
		m[k] = value(k, v)
	}
	return m
}

// Len returns the number of environment variables defined in e.
func (e Env) Len() int { return len(e.env) }

// Clone creates a new Env instance that is identical to, but independent from,
// e.
func (e Env) Clone() Env {
	var clone Env
	if len(e.env) > 0 {
		clone.env = make(map[string]string, len(e.env))
		for k, v := range e.env {
			clone.env[k] = v
		}
	}
	return clone
}

// Split splits the supplied environment variable value into a key/value pair.
//
// If v is of the form:
//	- KEY, returns (KEY, "")
//	- KEY=, returns (KEY, "")
//	- KEY=VALUE, returns (KEY, VALUE)
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

// getValue returns the value from an internal "env" map value field.
//
// It assumes that the environment variable is well-formed, one of:
// - KEY
// - KEY=
// - KEY=VALUE
func value(key, v string) string {
	prefixSize := len(key) + len("=")
	if len(v) <= prefixSize {
		return ""
	}
	return v[prefixSize:]
}

// Join creates an environment variable definition for the supplied key/value.
func Join(k, v string) string {
	if v == "" {
		return k
	}
	return strings.Join([]string{k, v}, "=")
}
