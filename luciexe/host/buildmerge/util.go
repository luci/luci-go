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

package buildmerge

import (
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/common/types"
)

// absolutizeURLs absolutizes the provided `logURL` and `viewURL`.
//
// No-op if both `logURL` and `viewURL` are absolute urls. Absolute url here
// means url that has valid scheme. A valid scheme must be
// `[a-zA-Z][a-zA-Z0-9+-.]*“ according to
// https://datatracker.ietf.org/doc/html/rfc3986#section-3.1
//
// If `logURL` is relative, calculate the return urls using `calcFn`. The
// provided `viewURL` will be omitted.
//
// Returns error if any of the following conditions are true:
//   - `logURL` is an absolute url but `viewURL` is empty.
//   - `logURL` is an absolute url but `viewURL` is not an absolute url.
//   - `logURL` is a relative url but not a valid logdog stream.
//
// The input `logURL` and `viewURL` will be returned verbatim on errors.
func absolutizeURLs(logURL, viewURL string, ns types.StreamName, calcFn CalcURLFn) (retLogURL, retViewURL string, err error) {
	retLogURL, retViewURL = logURL, viewURL
	if isAbs(logURL) {
		switch {
		case viewURL == "":
			return retLogURL, retViewURL, errors.Fmt("absolute log url is provided %q but view url is empty", logURL)
		case !isAbs(viewURL):
			return retLogURL, retViewURL, errors.Fmt("expected absolute view url, got %q", viewURL)
		}
		// both urls are absolute.
		return retLogURL, retViewURL, nil
	}

	stream := types.StreamName(logURL)
	if serr := stream.Validate(); serr != nil {
		return retLogURL, retViewURL, errors.Fmt("bad log url %q: %w", logURL, serr)
	}
	retLogURL, retViewURL = calcFn(ns, stream)
	return retLogURL, retViewURL, nil
}

func isAbs(url string) bool {
	idx := strings.Index(url, "://")
	if idx <= 0 {
		return false
	}
	scheme := url[:idx]
	for i := 0; i < len(scheme); i++ {
		c := scheme[i]
		switch {
		case 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z':
		// always valid
		case '0' <= c && c <= '9' || c == '+' || c == '-' || c == '.':
			if i != 0 {
				return false
			}
		}
	}
	return true
}
