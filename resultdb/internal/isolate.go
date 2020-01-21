// Copyright 2019 The LUCI Authors.
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

package internal

import (
	"fmt"
	"regexp"

	"go.chromium.org/luci/common/errors"
)

// IsolateURL returns a machine-readable URL for an isolated object.
func IsolateURL(host, ns, digest string) string {
	return fmt.Sprintf("isolate://%s/%s/%s", host, ns, digest)
}

var isolateURLre = regexp.MustCompile(`^isolate://([^/]+)/([^/]+)/(.+)`)

// ParseIsolateURL parses an isolate URL. It is a reverse of IsolateURL.
func ParseIsolateURL(s string) (host, ns, digest string, err error) {
	m := isolateURLre.FindStringSubmatch(s)
	if m == nil {
		err = errors.Reason("does not match %s", isolateURLre).Err()
		return
	}

	host = m[1]
	ns = m[2]
	digest = m[3]
	return
}
