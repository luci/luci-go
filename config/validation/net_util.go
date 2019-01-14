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

package validation

import (
	"fmt"
	"regexp"
	"strings"
)

const hostnameSegmentPattern = "[a-z0-9]([a-z0-9-]*[a-z0-9])*"

var hostnameRegexp = regexp.MustCompile(fmt.Sprintf(
	"^%s([.]%s)*$",
	hostnameSegmentPattern,
	hostnameSegmentPattern,
))

// ValidateHostname returns an error if the given string is not a valid
// RFC1123 hostname.
func ValidateHostname(hostname string) error {
	if len(hostname) > 255 {
		return fmt.Errorf("length exceeds 255 bytes")
	}
	if !hostnameRegexp.MatchString(hostname) {
		return fmt.Errorf("hostname does not match regex %q", hostnameRegexp)
	}
	for _, s := range strings.Split(hostname, ".") {
		if len(s) > 63 {
			return fmt.Errorf("segment %q exceeds 63 bytes", s)
		}
	}
	return nil
}
