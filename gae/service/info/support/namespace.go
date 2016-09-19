// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
