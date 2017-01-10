// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cfgclient

import (
	"fmt"
	"sync"
)

// Formatter applies a transformation to the data's cached representation.
//
// The Formatter is supplied with the content to format along with the
// formatter-specific metadata, fd.
//
// Formatter operates on a backend.Item's Content via the "format" Backend.
type Formatter interface {
	FormatItem(c, fd string) (string, error)
}

// FormatterRegistry is a registry of Resolver key mapped to the Formatter to
// use to format that key.
type FormatterRegistry struct {
	registryMu sync.RWMutex
	registry   map[string]Formatter
}

// Register registers a Formatter with the FormatBackend.
//
// If the supplied key is already registered, Register will panic.
func (r *FormatterRegistry) Register(rk string, f Formatter) {
	if rk == "" {
		panic("cannot register empty key")
	}

	r.registryMu.Lock()
	defer r.registryMu.Unlock()

	if _, ok := r.registry[rk]; ok {
		panic(fmt.Errorf("key %q is already registered", rk))
	}
	if r.registry == nil {
		r.registry = map[string]Formatter{}
	}
	r.registry[rk] = f
}

// Get returns the Formatter associated with the provided Format.
func (r *FormatterRegistry) Get(f string) Formatter {
	r.registryMu.RLock()
	defer r.registryMu.RUnlock()
	return r.registry[f]
}
