// Copyright 2023 The LUCI Authors.
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

// Package validate contains validation for RPC requests and Config entries.
package validate

import (
	"regexp"
	"strings"

	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

var cipdExpander = template.DefaultExpander()

const (
	maxDimensionKeyLen = 64
	maxDimensionValLen = 256
	// Max length of a valid service account.
	maxServiceAccountLength = 128
	// Maximum acceptable priority value, which is effectively the lowest priority.
	maxPriority = 255
	// If no update is received from bot after max seconds lapsed from its last
	// ping to the server, it will be considered dead.
	maxBotPingTolanceSecs = 1200
	// Min time to keep the bot alive before it is declared dead.
	minBotPingTolanceSecs = 60
)

var (
	dimensionKeyRe   = regexp.MustCompile(`^[a-zA-Z\-\_\.][0-9a-zA-Z\-\_\.]*$`)
	reservedTags     = stringset.NewFromSlice([]string{"swarming.terminate"}...)
	serviceAccountRE = regexp.MustCompile(`^[0-9a-zA-Z_\-\.\+\%]+@[0-9a-zA-Z_\-\.]+$`)
)

// DimensionKey checks if `key` can be a dimension key.
func DimensionKey(key string) error {
	if err := keyLength(key); err != nil {
		return err
	}
	if !dimensionKeyRe.MatchString(key) {
		return errors.Reason("the key should match %s", dimensionKeyRe).Err()
	}
	return nil
}

func keyLength(key string) error {
	if key == "" {
		return errors.Reason("the key cannot be empty").Err()
	}
	if len(key) > maxDimensionKeyLen {
		return errors.Reason("the key should be no longer than %d (got %d)", maxDimensionKeyLen, len(key)).Err()
	}
	return nil
}

// DimensionValue checks if `val` can be a dimension value.
func DimensionValue(val string) error {
	if val == "" {
		return errors.Reason("the value cannot be empty").Err()
	}
	if len(val) > maxDimensionValLen {
		return errors.Reason("the value should be no longer than %d (got %d)", maxDimensionValLen, len(val)).Err()
	}
	if strings.TrimSpace(val) != val {
		return errors.Reason("the value should have no leading or trailing spaces").Err()
	}
	return nil
}

// CipdPackageName checks CIPD package name is correct.
//
// The package name is allowed to be a template e.g. have "${platform}" and
// other substitutions inside.
func CipdPackageName(pkg string) error {
	expanded, err := cipdExpander.Expand(pkg)
	if err != nil {
		return errors.Annotate(err, "bad package name template %q", pkg).Err()
	}
	if err := common.ValidatePackageName(expanded); err != nil {
		return err // the error has all details already
	}
	return nil
}

// CipdPackageVersion checks CIPD package version is correct.
func CipdPackageVersion(ver string) error {
	return common.ValidateInstanceVersion(ver)
}

// Tag checks a "<key>:<value>" tag is correct.
func Tag(tag string) error {
	parts := strings.Split(tag, ":")
	if len(parts) != 2 {
		return errors.Reason("tag must be in key:value form, not %q", tag).Err()
	}

	key, value := parts[0], parts[1]
	if err := keyLength(key); err != nil {
		return err
	}
	if err := DimensionValue(value); err != nil {
		return err
	}
	if reservedTags.Has(key) {
		return errors.Reason("tag %q is reserved for internal use and can't be assigned manually", key).Err()
	}
	return nil
}

// ServiceAccount checks a service account is correct.
func ServiceAccount(sa string) error {
	if len(sa) > maxServiceAccountLength {
		return errors.Reason("too long %q: %d > %d", sa, len(sa), maxServiceAccountLength).Err()
	}

	if !serviceAccountRE.MatchString(sa) {
		return errors.Reason("invalid %q: must be an email", sa).Err()
	}
	return nil
}

// Priority checks a priority is correct.
func Priority(p int32) error {
	if p < 0 || p > maxPriority {
		return errors.Reason("invalid %d, must be between 0 and %d", p, maxPriority).Err()
	}
	return nil
}

func BotPingTolerance(bpt int64) error {
	if bpt < minBotPingTolanceSecs || bpt > maxBotPingTolanceSecs {
		return errors.Reason("invalid %d, must be between %d and %d", bpt, minBotPingTolanceSecs, maxBotPingTolanceSecs).Err()
	}
	return nil
}
