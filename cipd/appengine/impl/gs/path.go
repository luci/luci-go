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

package gs

import (
	"fmt"
	"strings"
	"unicode"
)

// ValidatePath returns an error if p doesn't look like "/bucket/path".
//
// Additionally it verifies p is using only printable ASCII characters, since
// all Google Storage paths used by CIPD are ASCII (assembled from constants
// fetched from validated configs and hex digests, there are no user-supplied
// path elements). We want to assert no fancy unicode characters sneak in.
func ValidatePath(p string) error {
	chunks := strings.Split(p, "/")
	if len(chunks) < 3 || chunks[0] != "" {
		return fmt.Errorf("a Google Storage path must have format /<bucket>/<path>")
	}
	for i := 1; i < len(chunks); i++ {
		if chunks[i] == "" {
			return fmt.Errorf("a Google Storage path components must not be empty")
		}
	}
	for _, r := range p {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return fmt.Errorf("forbidden symbol %q in the google storage path", r)
		}
	}
	return nil
}

// SplitPath given "/a/b/c" returns ("a", "b/c") or panics.
//
// Use ValidatePath for prior validation if you are concerned.
func SplitPath(p string) (bucket, path string) {
	if err := ValidatePath(p); err != nil {
		panic(err)
	}
	sep := strings.IndexByte(p[1:], '/') + 1
	return p[1:sep], p[sep+1:]
}
