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

// Package support provides Info-related support functionality. It is designed
// to be used by implementing packages.
package support

import (
	"fmt"
	"regexp"
)

// validNamespace matches valid namespace names.
var validNamespace = regexp.MustCompile(`^[0-9A-Za-z._-]{0,100}$`)

// ValidNamespace will return an error if the supplied string is not a valid
// namespace.
func ValidNamespace(ns string) error {
	if validNamespace.MatchString(ns) {
		return nil
	}
	return fmt.Errorf("appengine: namespace %q does not match /%s/", ns, validNamespace)
}
