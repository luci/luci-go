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

// Package botstate implements handling of Swarming bot state dictionary.
package botstate

import (
	"bytes"
	"encoding/json"

	"go.chromium.org/luci/gae/service/datastore"
)

// Dict is a lazily deserialized read-only bot state JSON dict.
//
// It can be in three state internally:
//  1. Sealed. In this state it is represented exclusively by a JSON byte blob
//     (which may or may not be valid).
//  2. Unsealed. In this state the dict is also accessible as a string-keyed
//     map, allowing to read values of individual keys.
//  3. Broken. This happens when there's an error deserializing the JSON byte
//     blob when unsealing it. Attempts to read keys of a broken dict will
//     end in errors.
//
// Reading values unseals the dict. It can also be unsealed explicitly via
// Unseal method.
type Dict struct {
	// JSON is serialized JSON representation of the dict.
	JSON []byte

	dict   map[string]json.RawMessage // non-nil when unsealed
	broken error                      // set if JSON could not be deserialized
}

// String is used for debug-logging the dict.
func (d *Dict) String() string {
	if len(d.JSON) == 0 {
		return "<empty>"
	}
	return string(d.JSON)
}

// Equal returns true if both dicts serialize to the same JSON byte blob.
//
// This method is used by github.com/google/go-cmp/cmp in assertions. For that
// reason it has to have a non-pointer receiver.
func (d Dict) Equal(another Dict) bool {
	a, _ := d.MarshalJSON()
	b, _ := d.MarshalJSON()
	return bytes.Equal(a, b)
}

// Unseal deserializes the JSON blob if it hasn't been deserialized yet.
func (d *Dict) Unseal() error {
	switch {
	case d.broken != nil:
		return d.broken
	case d.dict != nil:
		return nil // already successfully unsealed
	}
	dict := map[string]json.RawMessage{}
	if len(d.JSON) != 0 {
		if d.broken = json.Unmarshal(d.JSON, &dict); d.broken != nil {
			return d.broken
		}
	}
	d.dict = dict
	return nil
}

// Err returns an error if the state dict is broken (not a valid JSON).
func (d *Dict) Err() error {
	return d.Unseal()
}

// ToProperty is a part of datastore.PropertyConverter interface.
func (d *Dict) ToProperty() (datastore.Property, error) {
	var prop datastore.Property
	err := prop.SetValue(d.JSON, datastore.NoIndex)
	return prop, err
}

// FromProperty is a part of datastore.PropertyConverter interface.
func (d *Dict) FromProperty(prop datastore.Property) error {
	blob, err := prop.Project(datastore.PTBytes)
	if err != nil {
		return err
	}
	*d = Dict{JSON: blob.([]byte)}
	return nil
}

// MarshalJSON implements json.Marshaler interface.
func (d *Dict) MarshalJSON() ([]byte, error) {
	if len(d.JSON) == 0 {
		return []byte(`{}`), nil
	}
	return d.JSON, nil
}

// UnmarshalJSON implements json.Unmarshaler interface.
func (d *Dict) UnmarshalJSON(blob []byte) error {
	// Per json.Unmarshaler doc, UnmarshalJSON must copy the JSON data if it
	// wishes to retain the data after returning.
	*d = Dict{JSON: bytes.Clone(blob)}
	return nil
}

// ReadRaw returns a raw serialized value of a key or nil if the key is missing.
//
// Returns an error if the dict can't be deserialized.
func (d *Dict) ReadRaw(key string) (json.RawMessage, error) {
	if err := d.Unseal(); err != nil {
		return nil, err
	}
	raw, ok := d.dict[key]
	if !ok {
		return nil, nil
	}
	return raw, nil
}

// Read reads a value of the given key if it is present.
//
// Doesn't change `val` and returns nil if the key is missing. Returns an error
// if the dict or the key value can't be deserialized.
func (d *Dict) Read(key string, val any) error {
	switch raw, err := d.ReadRaw(key); {
	case err != nil:
		return err
	case raw == nil:
		return nil
	default:
		return json.Unmarshal(raw, val)
	}
}

// MustReadBool reads a boolean, returning false if the key is missing or on
// errors.
func (d *Dict) MustReadBool(key string) bool {
	var val bool
	if err := d.Read(key, &val); err != nil {
		return false
	}
	return val
}

// MustReadString reads a string, returning an empty string if the key is
// missing or on errors.
func (d *Dict) MustReadString(key string) string {
	var val string
	if err := d.Read(key, &val); err != nil {
		return ""
	}
	return val
}
