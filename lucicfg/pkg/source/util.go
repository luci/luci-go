// Copyright 2025 The LUCI Authors.
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

package source

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"path"
)

// NormalizePkgRoot checks that `pkgRoot` matches its path.Clean equivalent.
//
// Normalizes "/" and "." to "".
func NormalizePkgRoot(pkgRoot string) (string, error) {
	if pkgRoot != "" {
		if cleaned := path.Clean(pkgRoot); cleaned != pkgRoot {
			return "", fmt.Errorf("pkgRoot is not clean: %q (cleaned: %q)", pkgRoot, cleaned)
		}
	}

	// pkgRoot does not end with a "/" unless it is literally "/"
	// pkgRoot of "." is a bad way to spell ""
	if pkgRoot == "." || pkgRoot == "/" {
		pkgRoot = ""
	}

	return pkgRoot, nil
}

// HashString returns the base64 encoded sha256 of `value`.
func HashString(value string) string {
	h := sha256.New()
	io.WriteString(h, value)
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
