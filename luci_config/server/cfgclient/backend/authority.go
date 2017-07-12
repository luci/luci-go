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

package backend

import (
	"bytes"

	"github.com/luci/luci-go/common/errors"
)

// Authority is the authority that is requesting configurations. It can be
// installed via WithAuthority.
//
// Authority marshals/unmarshals to/from a compact JSON representation. This is
// used by the caching layer.
type Authority int

const (
	// AsAnonymous requests config data as an anonymous user.
	//
	// Corresponds to auth.NoAuth.
	AsAnonymous Authority = iota

	// AsService requests config data as the currently-running service.
	//
	// Corresponds to auth.AsSelf.
	AsService

	// AsUser requests config data as the currently logged-in user.
	//
	// Corresponds to auth.AsUser.
	AsUser
)

var (
	asAnonymousJSON = []byte(`""`)
	asServiceJSON   = []byte(`"S"`)
	asUserJSON      = []byte(`"U"`)
)

// MarshalJSON implements encoding/json.Marshaler.
//
// We use a shorthand notation so that we don't waste space in JSON.
func (a Authority) MarshalJSON() ([]byte, error) {
	switch a {
	case AsAnonymous:
		return asAnonymousJSON, nil
	case AsService:
		return asServiceJSON, nil
	case AsUser:
		return asUserJSON, nil
	default:
		return nil, errors.Reason("unknown authority: %v", a).Err()
	}
}

// UnmarshalJSON implements encoding/json.Unmarshaler.
func (a *Authority) UnmarshalJSON(d []byte) error {
	switch {
	case bytes.Equal(d, asAnonymousJSON):
		*a = AsAnonymous
	case bytes.Equal(d, asServiceJSON):
		*a = AsService
	case bytes.Equal(d, asUserJSON):
		*a = AsUser
	default:
		return errors.Reason("unknown authority JSON value: %v", d).Err()
	}
	return nil
}
