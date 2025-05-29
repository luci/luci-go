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

package types

import (
	"flag"
	"net/url"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
)

const logDogURLScheme = "logdog"

// StreamAddr is a fully-qualified LogDog stream address.
type StreamAddr struct {
	// Host is the LogDog host.
	Host string `json:"host,omitempty"`

	// Project is the LUCI project name that this log belongs to.
	Project string `json:"project,omitempty"`

	// Path is the LogDog stream path.
	Path StreamPath `json:"path,omitempty"`
}

var _ flag.Value = (*StreamAddr)(nil)

// Set implements flag.Value
func (s *StreamAddr) Set(v string) error {
	a, err := ParseURL(v)
	if err != nil {
		return err
	}
	*s = *a
	return nil
}

// Validate returns an error if the supplied StreamAddr is not valid.
func (s *StreamAddr) Validate() error {
	if s.Host == "" {
		return errors.New("cannot have empty Host")
	}
	if err := config.ValidateProjectName(s.Project); err != nil {
		return err
	}
	if err := s.Path.Validate(); err != nil {
		return err
	}
	return nil
}

// IsZero returns true iff all fields are empty.
func (s *StreamAddr) IsZero() bool {
	return s.Host == "" && s.Path == "" && s.Project == ""
}

// String returns a string representation of this address.
func (s *StreamAddr) String() string { return s.URL().String() }

// URL returns a LogDog URL that represents this Stream.
func (s *StreamAddr) URL() *url.URL {
	return &url.URL{
		Scheme: logDogURLScheme,
		Host:   s.Host,
		Path:   strings.Join([]string{"", string(s.Project), string(s.Path)}, "/"),
	}
}

// ParseURL parses a LogDog URL into a Stream. If the URL is malformed, or
// if the host, project, or path is invalid, an error will be returned.
//
// A LogDog URL has the form:
// logdog://<host>/<project>/<prefix>/+/<name>
func ParseURL(v string) (*StreamAddr, error) {
	u, err := url.Parse(v)
	if err != nil {
		return nil, errors.Fmt("failed to parse URL: %w", err)
	}

	// Validate Scheme.
	if u.Scheme != logDogURLScheme {
		return nil, errors.Fmt("URL scheme %q is not %s", u.Scheme, logDogURLScheme)
	}
	addr := StreamAddr{
		Host: u.Host,
	}

	parts := strings.SplitN(u.Path, "/", 3)
	if len(parts) != 3 || len(parts[0]) != 0 {
		return nil, errors.Fmt("URL path does not include both project and path components: %s", u.Path)
	}

	addr.Project, addr.Path = parts[1], StreamPath(parts[2])
	if err := config.ValidateProjectName(addr.Project); err != nil {
		return nil, errors.Fmt("invalid project name: %q: %w", addr.Project, err)
	}

	if err := addr.Path.Validate(); err != nil {
		return nil, errors.Fmt("invalid stream path: %q: %w", addr.Path, err)
	}

	return &addr, nil
}
