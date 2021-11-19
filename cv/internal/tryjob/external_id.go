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

package tryjob

import (
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// ExternalID is a unique ID deterministically constructed to identify tryjobs.
//
// Currently, only Buildbucket is supported.
type ExternalID string

// BuildbucketID makes an ExternalID for a Buildbucket build.
//
// Host is typically "cr-buildbucket.appspot.com".
// Build is a number, e.g. 8839722009404151168 for
// https://ci.chromium.org/ui/p/infra/builders/try/infra-try-bionic-64/b8839722009404151168/overview
func BuildbucketID(host string, build int64) (ExternalID, error) {
	if strings.ContainsRune(host, '/') {
		return "", errors.Reason("invalid host %q: must not contain /", host).Err()
	}
	return ExternalID(fmt.Sprintf("buildbucket/%s/%d", host, build)), nil
}

// MustBuildbucketID is like `BuildbucketID()` but panics on error.
func MustBuildbucketID(host string, build int64) ExternalID {
	ret, err := BuildbucketID(host, build)
	if err != nil {
		panic(err)
	}
	return ret
}

// ParseBuildbucketID returns Buildbucket host and build if this is a
// BuildbucketID.
func (e ExternalID) ParseBuildbucketID() (host string, build int64, err error) {
	parts := strings.Split(string(e), "/")
	if len(parts) != 3 || parts[0] != "buildbucket" {
		err = errors.Reason("%q is not a valid BuildbucketID", e).Err()
		return
	}
	host = parts[1]
	build, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		err = errors.Annotate(err, "%q is not a valid BuildbucketID", e).Err()
	}
	return
}

// URL returns URL of the Tryjob.
func (e ExternalID) URL() (string, error) {
	switch kind, err := e.kind(); {
	case err != nil:
		return "", err
	case kind == "buildbucket":
		host, build, err := e.ParseBuildbucketID()
		if err != nil {
			return "", errors.Annotate(err, "invalid tryjob.ExternalID").Err()
		}
		return fmt.Sprintf("https://%s/build/%d", host, build), nil
	default:
		return "", errors.Reason("unrecognized ExternalID: %q", e).Err()
	}
}

// MustURL is like `URL()` but panic on err.
func (e ExternalID) MustURL() string {
	ret, err := e.URL()
	if err != nil {
		panic(err)
	}
	return ret
}

func (e ExternalID) kind() (string, error) {
	s := string(e)
	idx := strings.IndexRune(s, '/')
	if idx <= 0 {
		return "", errors.Reason("invalid ExternalID: %q", s).Err()
	}
	return s[:idx], nil
}
