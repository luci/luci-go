// Copyright 2021 The LUCI Authors.
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

package gerrit

import (
	"net/url"
	"regexp"
	"strconv"

	"go.chromium.org/luci/common/errors"
)

var regexCrRevPath = regexp.MustCompile(`/([ci])/(\d+)(/(\d+))?`)
var regexGoB = regexp.MustCompile(`((\w+-)+review\.googlesource\.com)/(#/)?(c/)?(([^\+]+)/\+/)?(\d+)(/(\d+)?)?`)

// FuzzyParseURL attempts to match the given URL to a Gerrit change, as
// identified by a host and change number.
//
// NOTE that this is not guaranteed to be correct, as there is room for some
// ambiguity in Gerrit URLs
func FuzzyParseURL(s string) (string, int64, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", 0, err
	}
	var host string
	var change int64
	if u.Host == "crrev.com" {
		m := regexCrRevPath.FindStringSubmatch(u.Path)
		if m == nil {
			return "", 0, errors.New("invalid crrev.com URL")
		}
		switch m[1] {
		case "c":
			host = "chromium-review.googlesource.com"
		case "i":
			host = "chrome-internal-review.googlesource.com"
		default:
			panic("impossible")
		}
		if change, err = strconv.ParseInt(m[2], 10, 64); err != nil {
			return "", 0, errors.Fmt("invalid crrev.com URL change number /%s/", m[2])
		}
	} else {
		m := regexGoB.FindStringSubmatch(s)
		if m == nil {
			return "", 0, errors.Fmt("Gerrit URL didn't match regexp %q", regexGoB.String())
		}
		if host = m[1]; host == "" {
			return "", 0, errors.New("invalid Gerrit host")
		}
		if change, err = strconv.ParseInt(m[7], 10, 64); err != nil {
			return "", 0, errors.Fmt("invalid Gerrit URL change number /%s/", m[7])
		}
	}
	return host, change, nil
}
