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

package nestedflagset

import (
	"strings"
)

// Token is a single name[=value] string.
type token string

// split breaks a token into its name and value components.
func (t token) split() (name, value string) {
	split := strings.SplitN(string(t), "=", 2)
	name = split[0]
	if len(split) == 2 {
		value = split[1]
	}
	return
}
