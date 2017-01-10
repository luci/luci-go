// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
		return nil, errors.Reason("unknown GetAllTarget: %(value)v").D("value", gat).Err()
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
		return errors.Reason("unknown GetAllTarget: %(value)q").D("value", d).Err()
	}
	return nil
}
