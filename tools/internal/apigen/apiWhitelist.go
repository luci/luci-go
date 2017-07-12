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

package apigen

import (
	"errors"
	"flag"
	"strings"
)

// apiWhitelist is a flag.Value type that appends to the list on add.
type apiWhitelist []string

var _ flag.Value = (*apiWhitelist)(nil)

func (w *apiWhitelist) String() string {
	if *w == nil {
		return ""
	}
	return strings.Join(*w, ",")
}

func (w *apiWhitelist) Set(s string) error {
	if len(s) == 0 {
		return errors.New("cannot whitelist empty string")
	}

	*w = append(*w, s)
	return nil
}
