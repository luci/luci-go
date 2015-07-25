// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gcps

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// validMidResourceRunes is the set of runes that a resource may contain, not
// including letters and numbers.
const validMidResourceRunes = "-_.~+%"

// resourcePath constructs the resource's endpoint path.
func resourcePath(project, collection, name string) string {
	return fmt.Sprintf("projects/%s/%s/%s", project, collection, name)
}

// validateResource validates a resource name. Resource naming is described in:
// https://cloud.google.com/pubsub/overview#names
//
// As of 'v1', a resource must:
// - start with a letter.
// - end with a lowercase letter or number.
// - contain only letters, numbers, dashes (-), underscores (_) periods (.),
//   tildes (~), pluses (+), or percent signs (%).
// - be between 3 and 255 characters in length.
// - cannot begin with the string goog.
//
func validateResource(s string) error {
	if l := len(s); l < 3 || l > 255 {
		return fmt.Errorf("length (%d) must be between 3 and 255", l)
	}

	if strings.HasPrefix(s, "goog") {
		return errors.New("resource cannot begin with 'goog'")
	}

	// Validate correctness.
	for i, r := range s {
		if r >= unicode.MaxASCII {
			return fmt.Errorf("non-ASCII character found at index #%d", i)
		}

		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			// Must begin with an ASCII letter?
			if i == 0 {
				return errors.New("gcps: resource names must begin with a letter")
			}

			// Is this a valid mid-resource value?
			if !((r >= '0' && r <= '9') || strings.ContainsRune(validMidResourceRunes, r)) {
				return fmt.Errorf("gcps: invalid resource rune at %d: %c", i, r)
			}
		}
	}
	return nil
}
