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
	"net/url"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/common/types"
)

// absolutizeURLs absolutizes the provided `logURL` and `viewURL`.
//
// No-op if both `logURL` and `viewURL` are valid absolute urls. If `logURL` is
// relative, calculate the return urls using `calcFn`. The provided `viewURL`
// will be omitted.
//
// Returns error if any of the following conditions are true:
//  * `logURL` is not a valid url.
//  * `viewURL` is not a valid url when `logURL` is an absolute url.
//  * `viewURL` is not an absolute valid url when `logURL` is an absolute url.
//  * `logURL` is a valid relative url but not a valid logdog stream.
// The input `logURL` and `viewURL` will be returned verbatim on errors.
func absolutizeURLs(logURL, viewURL string, ns types.StreamName, calcFn CalcURLFn) (retLogURL, retViewURL string, err error) {
	retLogURL, retViewURL = logURL, viewURL
	switch parsedURL, uerr := url.Parse(logURL); {
	case uerr != nil:
		return retLogURL, retViewURL, errors.Annotate(uerr, "bad log url %q", logURL).Err()
	case parsedURL.Scheme != "":
		switch parsedViewURL, verr := url.Parse(viewURL); {
		case verr != nil:
			return retLogURL, retViewURL, errors.Annotate(verr, "bad view url %q", viewURL).Err()
		case parsedViewURL.Scheme == "":
			return retLogURL, retViewURL, errors.Reason("expected absolute view url, got %q", viewURL).Err()
		}
		// both urls are already absolute.
		return retLogURL, retViewURL, nil
	default:
		stream := types.StreamName(logURL)
		if serr := stream.Validate(); serr != nil {
			return retLogURL, retViewURL, errors.Annotate(serr, "bad log url %q", logURL).Err()
		}
		retLogURL, retViewURL = calcFn(ns, stream)
		return retLogURL, retViewURL, nil
	}
}
