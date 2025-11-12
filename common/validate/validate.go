// Copyright 2024 The LUCI Authors.
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

// Package validate contains methods for validating inputs.
package validate

import (
	"regexp"
	"regexp/syntax"
	"strings"

	"go.chromium.org/luci/common/errors"
)

var (
	// unspecified is the error to be used when something is unspecified when it's
	// supposed to.
	unspecified = errors.New("unspecified")

	// doesNotMatchRe is the error to be used when a string does not match a regex.
	doesNotMatchRe = errors.New("does not match")
)

// Unspecified returns an error indicating that a value is unspecified.
func Unspecified() error {
	return unspecified
}

// DoesNotMatchReErr returns an error indicating that a value does not match a
// regex.
func DoesNotMatchReErr(r *regexp.Regexp) error {
	return errors.Fmt("does not match pattern %q: %w", r, doesNotMatchRe)
}

// Regexp returns a non-nil error if re is not a valid regular
// expression.
func Regexp(re string) error {
	// Note: regexp.Compile uses syntax.Perl.
	_, err := syntax.Parse(re, syntax.Perl)
	return err
}

// RegexpFragment returns a non-nil error if re is not a valid regular
// expression fragment that can be embedded in a regex template.
func RegexpFragment(re string) error {
	if err := Regexp(re); err != nil {
		return err
	}

	// Do not allow ^ and $ in the regexp, because we need to be able to embed the
	// the user-supplied pattern in a regex template.
	//
	// TODO: we should improve this as checking ^ and $ is not sufficient.
	// e.g. "()^$()" will pass the check yet it can not be embedded in a regex
	// template.
	if strings.HasPrefix(re, "^") {
		return errors.New("must not start with ^")
	}
	if strings.HasSuffix(re, "$") {
		return errors.New("must not end with $")
	}

	return nil
}

// SpecifiedWithRe validates a value is non-empty and matches the given re.
func SpecifiedWithRe(re *regexp.Regexp, value string) error {
	if value == "" {
		return unspecified
	}
	if !re.MatchString(value) {
		return DoesNotMatchReErr(re)
	}
	return nil
}

// MatchReWithLength validates a value matches the given re and its length is
// within the specific range [minLen, maxLen].
//
// For convenience, if minLen > 0 and value == "", returns "unspecified" as the
// error message (instead of referring to length limit in the error message).
func MatchReWithLength(re *regexp.Regexp, minLen, maxLen int, value string) error {
	if value == "" && minLen > 0 {
		return unspecified
	}

	if len(value) < minLen {
		return errors.Fmt("must be at least %d bytes", minLen)
	}
	if len(value) > maxLen {
		return errors.Fmt("must be at most %d bytes", maxLen)
	}

	if value == "" {
		return unspecified
	}
	if !re.MatchString(value) {
		return DoesNotMatchReErr(re)
	}
	return nil
}
