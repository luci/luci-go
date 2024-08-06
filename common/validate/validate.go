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
	unspecified = errors.Reason("unspecified").Err()

	// doesNotMatchRe is the error to be used when a string does not match a regex.
	doesNotMatchRe = errors.Reason("does not match").Err()
)

func Unspecified() error {
	return unspecified
}

func DoesNotMatchReErr(r *regexp.Regexp) error {
	return errors.Annotate(doesNotMatchRe, "does not match pattern %q", r).Err()
}

// RegexpFragment returns a non-nil error if re is not a valid regular
// expression fragment that can be embedded in a regex template.
func RegexpFragment(re string) error {
	// Note: regexp.Compile uses syntax.Perl.
	if _, err := syntax.Parse(re, syntax.Perl); err != nil {
		return err
	}

	// Do not allow ^ and $ in the regexp, because we need to be able to embed the
	// the user-supplied pattern in a regex template.
	//
	// TODO: we should improve this as checking ^ and $ is not sufficient.
	// e.g. "()^$()" will pass the check yet it can not be embedded in a regex
	// template.
	if strings.HasPrefix(re, "^") {
		return errors.Reason("must not start with ^").Err()
	}
	if strings.HasSuffix(re, "$") {
		return errors.Reason("must not end with $").Err()
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
