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

// GetAllTarget is the type of configuration to retrieve with GetAll.
//
// GetAllTarget marshals/unmarshals to/from a compact JSON representation. This is
// used by the caching layer.
type GetAllTarget string

const (
	// GetAllProject indicates that project configus should be retrieved.
	GetAllProject = GetAllTarget("Project")
	// GetAllRef indicates that ref configs should be retrieved.
	GetAllRef = GetAllTarget("Ref")
)

var (
	projectJSON = []byte(`"P"`)
	refJSON     = []byte(`"R"`)
)

// MarshalJSON implements json.Marshaler.
func (gat GetAllTarget) MarshalJSON() ([]byte, error) {
	switch gat {
	case GetAllProject:
		return projectJSON, nil
	case GetAllRef:
		return refJSON, nil
	default:
		return nil, errors.Reason("unknown GetAllTarget: %v", gat).Err()
	}
}

// UnmarshalJSON implements json.Unmarshaler.
func (gat *GetAllTarget) UnmarshalJSON(d []byte) error {
	switch {
	case bytes.Equal(d, projectJSON):
		*gat = GetAllProject
	case bytes.Equal(d, refJSON):
		*gat = GetAllRef
	default:
		return errors.Reason("unknown GetAllTarget: %q", d).Err()
	}
	return nil
}
