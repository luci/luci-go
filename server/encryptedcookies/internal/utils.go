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

package internal

import (
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
)

// NormalizeURL verifies URL is parsable and that it is relative.
func NormalizeURL(dest string) (string, error) {
	if dest == "" {
		return "/", nil
	}
	u, err := url.Parse(dest)
	if err != nil {
		return "", errors.Fmt("bad destination URL %q: %w", dest, err)
	}
	// Note: '//host/path' is a location on a server named 'host'.
	if u.IsAbs() || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "", errors.Fmt("bad absolute destination URL %q", u)
	}
	// path.Clean removes trailing slash. It matters for URLs though. Keep it.
	keepSlash := strings.HasSuffix(u.Path, "/")
	u.Path = path.Clean(u.Path)
	if !strings.HasSuffix(u.Path, "/") && keepSlash {
		u.Path += "/"
	}
	if !strings.HasPrefix(u.Path, "/") {
		return "", errors.Fmt("bad destination URL %q", u)
	}
	return u.String(), nil
}

// MakeRedirectURL is used to generate login and logout URLs.
func MakeRedirectURL(base, dest string) (string, error) {
	dest, err := NormalizeURL(dest)
	if err != nil {
		return "", err
	}
	if dest == "/" {
		return base, nil
	}
	v := url.Values{"r": {dest}}
	return base + "?" + v.Encode(), nil
}

// RemoveCookie sets a cookie to a past expiration date so that the browser can
// remove it.
//
// It also replaces the value with junk, in unlikely case the browser decides
// to ignore the expiration time.
func RemoveCookie(rw http.ResponseWriter, r *http.Request, cookie, path string) {
	http.SetCookie(rw, &http.Cookie{
		Name:    cookie,
		Value:   "deleted",
		Path:    path,
		MaxAge:  -1,
		Expires: time.Unix(1, 0),
	})
}
