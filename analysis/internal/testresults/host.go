// Copyright 2022 The LUCI Authors.
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

package testresults

import (
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Format of valid gerrit hostnames. We generally allow anything allowed in
// https://www.rfc-editor.org/rfc/rfc1123#page-13, except for a hostname
// without dots, as that conflicts with our compression scheme.
var hostnameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]+(\.[a-z0-9-]+)+$`)

// ValidateGerritHostname validates the given gerrit hostname.
func ValidateGerritHostname(host string) error {
	if !hostnameRE.MatchString(host) {
		return errors.Reason("gerrit hostname %q does not match expected pattern", host).Err()
	}
	if len(host) > 255 {
		return errors.Reason("gerrit hostname is longer than 255 characters").Err()
	}

	// If the hostname ends with -review.googlesource.com, then the
	// front part of the hostname must not contain a '.'.
	if strings.HasSuffix(host, GerritHostnameSuffix) {
		trimmedHost := strings.TrimSuffix(host, GerritHostnameSuffix)
		if strings.Contains(trimmedHost, ".") {
			return errors.Reason("bad hostname ending with -review.googlesource.com").Err()
		}
	}
	return nil
}

// CompressHost transforms a gerrit hostname into its compressed database
// representation.
func CompressHost(host string) string {
	// Most gerrit hostnames are of the form <project>-review.googlesource.com.
	// Store these in the database without the suffix.
	if strings.HasSuffix(host, GerritHostnameSuffix) {
		trimmedHost := strings.TrimSuffix(host, GerritHostnameSuffix)
		if strings.Contains(trimmedHost, ".") {
			panic("invalid gerrit hostname, contains '.' after removing gerrit hostname suffix")
		}
		return trimmedHost
	}
	// Other hostnames.
	if !strings.Contains(host, ".") {
		panic("invalid gerrit hostname, does not contain '.'")
	}
	return host
}

// DecompressHost recovers a gerrit hostname from its compressed database
// representation.
func DecompressHost(host string) string {
	if strings.Contains(host, ".") {
		return host
	} else {
		return host + GerritHostnameSuffix
	}
}
