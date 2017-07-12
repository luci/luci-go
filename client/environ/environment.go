// Copyright 2015 The LUCI Authors.
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

package environ

import (
	"os"
	"strings"
)

// Environment is a key/value mapping of environment variables.
type Environment map[string]string

// Load loads an Environment from a series of KEY or KEY=VALUE pairs.
func Load(env []string) Environment {
	if len(env) == 0 {
		return nil
	}

	environ := make(Environment, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts[0]) > 0 {
			switch len(parts) {
			case 1:
				environ[parts[0]] = ""
			case 2:
				environ[parts[0]] = parts[1]
			}
		}
	}
	return environ
}

// Get returns the current Environment from os.Environ.
func Get() Environment {
	return Load(os.Environ())
}
