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

package main

import (
	"path/filepath"
	"unicode"

	"go.chromium.org/luci/common/errors"
)

type title string

func (t title) validate() error {
	if len(t) == 0 {
		return errors.New("cannot be empty")
	}

	idx := 0
	for _, r := range t {
		if !(unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-') {
			return errors.Reason("character at %d (%c) is not permitted in a title", idx, r).Err()
		}
		idx++
	}
	return nil
}

// titleFromConfigPath returns the title of a configuration item identified by
// the specified configuration file.
//
// If the file was not a valid config path, or the title was not valid, an error
// will be returned.
func titleFromConfigPath(path string) (title, error) {
	path = filepath.Base(path)
	if filepath.Ext(path) == configExt {
		t := title(path[:len(path)-len(configExt)])
		if err := t.validate(); err != nil {
			return "", err
		}
		return t, nil
	}
	return "", errors.Reason("missing config extension [" + configExt + "]").Err()
}
