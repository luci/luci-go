// Copyright 2025 The LUCI Authors.
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

// Package creds contains helpers for the Git credential protocol
//
// https://git-scm.com/docs/git-credential
package creds

import (
	"bufio"
	"io"
	"slices"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// An Attrs contains attributes per the git credential helper
// protocol relevant for ReAuth.
type Attrs struct {
	Protocol     string
	Host         string
	Path         string
	Capabilities []string
}

func (a Attrs) HasAuthtypeCapability() bool {
	return slices.Contains(a.Capabilities, "authtype")
}

// ReadAttrs reads attributes per the git credential helper protocol.
func ReadAttrs(r io.Reader) (*Attrs, error) {
	attrs := &Attrs{}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		k, v, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			return nil, errors.Fmt("read git credential attributes: invalid line %q", scanner.Text())
		}
		switch k {
		case "host":
			attrs.Host = v
		case "path":
			attrs.Path = v
		case "protocol":
			attrs.Protocol = v
		case "capability[]":
			setArrayValue(&attrs.Capabilities, v)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, errors.Fmt("read git credential attributes: %w", err)
	}
	return attrs, nil
}

// setArrayValue is a helper for setting GitCredAttrs array values.
func setArrayValue(field *[]string, v string) {
	if v == "" {
		*field = nil
	} else {
		*field = append(*field, v)
	}
}
