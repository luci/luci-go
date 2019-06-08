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

func hostRequiresSSL(host string) bool {
	host = strings.ToLower(host)
	return strings.HasSuffix(host, ".appspot.com")
}

// CheckURL ensures that the URL has a valid scheme, and that, if it is an
// appspot server, that it uses HTTPS.
//
// If no protocol is specified, the protocol defaults to https://.
func CheckURL(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" {
		s = "https://" + s
		u.Scheme = "https"
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("only http:// or https:// scheme is accepted")
	}
	if u.Scheme != "https" && hostRequiresSSL(u.Host) {
		return "", errors.New("only https:// scheme is accepted for appspot hosts, it can be omitted")
	}
	if _, err = url.Parse(s); err != nil {
		return "", err
	}
	return s, nil
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
