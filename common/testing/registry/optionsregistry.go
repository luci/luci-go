// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package registry provides a way to register options to cmp.Diff.
package registry

import (
	"sync"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

// globalOptionsRegistryMutex is the mutex governing globalOptionsRegistry.
var globalOptionsRegistryMutex sync.Mutex

// globalOptionsRegistry is the registry of global options that will get passed to
// typed.Diff and other similar things that eventually call cmp.Diff.
var globalOptionsRegistry = []cmp.Option{
	protocmp.Transform(),
}

// RegisterCmpOption registers an option to the registry.
//
// RegisterCmpOption will panic if the option is nil. This function does not
// check for duplicate options because I don't have a good way to do that.
func RegisterCmpOption(opt cmp.Option) {
	if opt == nil {
		panic("cannot register nil option")
	}
	globalOptionsRegistryMutex.Lock()
	defer globalOptionsRegistryMutex.Unlock()
	globalOptionsRegistry = append(globalOptionsRegistry, opt)
}

// GetCmpOptions gets a copy of the registry at the time it was called.
func GetCmpOptions() []cmp.Option {
	globalOptionsRegistryMutex.Lock()
	defer globalOptionsRegistryMutex.Unlock()
	return append([]cmp.Option{}, globalOptionsRegistry...)
}
