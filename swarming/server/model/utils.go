// Copyright 2023 The LUCI Authors.
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

package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/klauspost/compress/zlib"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// checkIsHex returns an error if the string doesn't look like a lowercase hex
// string.
func checkIsHex(s string, minLen int) error {
	if len(s) < minLen {
		return errors.New("too small")
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return errors.Reason("bad lowercase hex string %q, wrong char %c", s, c).Err()
		}
	}
	return nil
}

// ToJSONProperty serializes a value into a JSON blob property.
//
// Empty maps and lists are stored as nulls.
func ToJSONProperty(val any) (datastore.Property, error) {
	if val == nil {
		return datastore.MkPropertyNI(nil), nil
	}
	blob, err := json.Marshal(val)
	if bytes.Equal(blob, []byte("{}")) || bytes.Equal(blob, []byte("[]")) {
		return datastore.MkPropertyNI(nil), nil
	}
	return datastore.MkPropertyNI(string(blob)), err
}

// FromJSONProperty deserializes a JSON blob property into `val`.
//
// If the property is missing, `val` will be unchanged. Assumes `val` is a list
// or a dict.
//
// Recognizes zlib-compressed properties for compatibility with older entities.
func FromJSONProperty(prop datastore.Property, val any) error {
	propVal, err := prop.Project(datastore.PTBytes)
	if err != nil {
		return err
	}
	blob, _ := propVal.([]byte)
	if len(blob) == 0 {
		return nil
	}

	// This seems to be an uncompressed JSON. Load it as is. Note that zlib
	// compressed data always starts with 0x78 byte (part of the zlib header).
	if blob[0] == '{' || blob[0] == '[' {
		return json.Unmarshal(blob, val)
	}

	// If this doesn't look like JSON, this is likely an older zlib-compressed
	// property. Try to uncompress and load it.
	r, err := zlib.NewReader(bytes.NewBuffer(blob))
	if err != nil {
		return err
	}
	w := bytes.NewBuffer(nil)
	if _, err := io.Copy(w, r); err != nil {
		_ = r.Close()
		return err
	}
	if err := r.Close(); err != nil {
		return err
	}
	return json.Unmarshal(w.Bytes(), val)
}

// LegacyNullProperty is a placeholder for "recognizing" legacy properties.
//
// The python side still writes them, but with `null` value. If we just ignore
// them completely from the Go side, they'll end up in `Extra` maps, which we
// want to avoid (`Extra` is only for truly unexpected properties).
type LegacyNullProperty struct{}

var _ datastore.PropertyConverter = &LegacyNullProperty{}

// FromProperty implements datastore.PropertyConverter.
func (*LegacyNullProperty) FromProperty(p datastore.Property) error {
	if p.Type() != datastore.PTNull {
		return fmt.Errorf("expecting null, but got %v", p)
	}
	return nil
}

// ToProperty implements datastore.PropertyConverter.
func (*LegacyNullProperty) ToProperty() (datastore.Property, error) {
	return datastore.Property{}, datastore.ErrSkipProperty
}
