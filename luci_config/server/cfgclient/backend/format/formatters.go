// Copyright 2017 The LUCI Authors.
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

package format

import (
	"fmt"
	"sync"

	"go.chromium.org/luci/common/errors"
)

// registry is a registry of Resolver key mapped to the Formatter to use to
// format that key.
var registry struct {
	sync.RWMutex
	r map[string]Formatter
}

// Formatter applies a transformation to the data's cached representation.
//
// The Formatter is supplied with the content to format along with the
// formatter-specific metadata, fd.
//
// Formatter operates on a backend.Item's Content via the "format" Backend.
type Formatter interface {
	FormatItem(c, fd string) (string, error)
}

// Register registers a Formatter implementation for given format.
//
// If the supplied key is already registered, Register will panic.
func Register(rk string, f Formatter) {
	if rk == "" {
		panic("cannot register empty key")
	}

	registry.Lock()
	defer registry.Unlock()

	if _, ok := registry.r[rk]; ok {
		panic(fmt.Errorf("key %q is already registered", rk))
	}
	if registry.r == nil {
		registry.r = map[string]Formatter{}
	}
	registry.r[rk] = f
}

// ClearRegistry removes all registered formatters.
//
// Useful in tests that call Register to setup initial state.
func ClearRegistry() {
	registry.Lock()
	defer registry.Unlock()
	registry.r = nil
}

// getFormatter returns the Formatter associated with the provided Format.
func getFormatter(f string) (Formatter, error) {
	registry.RLock()
	defer registry.RUnlock()

	formatter := registry.r[f]
	if formatter == nil {
		return nil, errors.Reason("unknown formatter: %q", f).Err()
	}
	return formatter, nil
}
