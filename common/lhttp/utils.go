// Copyright 2015 The LUCI Authors.
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

package lhttp

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// ParseHostURL ensures that the URL has a valid scheme and that it uses
// `http://` only when the host is a localhost server.
//
// If no scheme is specified, the scheme defaults to `https://`.
//
// If the URL has a path component, it is silently stripped.
func ParseHostURL(s string) (*url.URL, error) {
	// If the URL doesn't have a scheme, url.Parse treats it as an opaque URL and
	// doesn't populate s.Host.
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, errors.New("can't extract host from the URL")
	}
	if u.Scheme == "" {
		u.Scheme = "https"
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, errors.New("only http:// or https:// scheme is accepted")
	}
	if u.Scheme == "http" && !IsLocalHost(u.Host) {
		return nil, errors.New("http:// can only be used with localhost servers")
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u, nil
}

// IsLocalHost returns true if hostport is local.
func IsLocalHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return false
	}
	switch {
	case host == "localhost", host == "":
	case net.ParseIP(host).IsLoopback():

	default:
		return false
	}
	return true
}
