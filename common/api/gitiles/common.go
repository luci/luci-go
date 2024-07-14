// Copyright 2016 The LUCI Authors.
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

package gitiles

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// OAuthScope is the OAuth 2.0 scope that must be included when acquiring an
// access token for Gitiles RPCs.
const OAuthScope = "https://www.googleapis.com/auth/gerritcodereview"

var hostnameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])*.googlesource.com$`)

// ValidateRepoHost validates gitiles host.
func ValidateRepoHost(host string) error {
	if !hostnameRe.MatchString(host) {
		return errors.Reason("hostname %s is not a valid Gitiles host", host).Err()
	}
	return nil
}

// ValidateRepoURL validates gitiles repository URL.
func ValidateRepoURL(repoURL string) error {
	_, _, err := ParseRepoURL(repoURL)
	return err
}

// ParseRepoURL parses a Gitiles repository URL.
func ParseRepoURL(repoURL string) (host, project string, err error) {
	var u *url.URL
	u, err = url.Parse(repoURL)
	switch {
	case err != nil:
		err = errors.Annotate(err, "invalid URL").Err()
	case u.Scheme != "https":
		err = errors.New("only https scheme is supported")
	case !strings.HasSuffix(u.Host, ".googlesource.com"):
		err = errors.New("only .googlesource.com repos are supported")
	case strings.Contains(u.Path, "+"):
		err = errors.New("repo URL path must not contain +")
	case !strings.HasPrefix(u.Path, "/"):
		err = errors.New("repo URL path must start with a slash")
	case u.Path == "/":
		err = errors.New("repo URL path must be not just a slash")
	case u.RawQuery != "":
		err = errors.New("repo URL must not have query")
	case u.Fragment != "":
		err = errors.New("repo URL must not have fragment")
	default:
		host = u.Host
		if strings.HasPrefix(u.Path, "/a/") {
			u.Path = strings.TrimPrefix(u.Path, "/a")
		}
		project = strings.Trim(u.Path, "/")
		project = strings.TrimSuffix(project, ".git")
	}
	return
}

// FormatRepoURL returns a canonical gitiles URL of the repo.
// If auth is true, the returned URL has "/a/" path prefix.
// See also ParseRepoURL.
func FormatRepoURL(host, project string, auth bool) url.URL {
	pathPrefix := ""
	if auth {
		pathPrefix = "/a"
	}
	return url.URL{
		Scheme: "https",
		Host:   host,
		Path:   fmt.Sprintf("%s/%s", pathPrefix, project),
	}
}

// NormalizeRepoURL is a shortcut for ParseRepoURL and FormatRepoURL.
func NormalizeRepoURL(repoURL string, auth bool) (*url.URL, error) {
	host, project, err := ParseRepoURL(repoURL)
	if err != nil {
		return nil, err
	}
	u := FormatRepoURL(host, project, auth)
	return &u, nil
}
