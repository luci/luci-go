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

package config

import (
	"encoding/json"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
)

// ToProperty converts a Header into a datastore.Property.
//
// This implements PropertyConverter. ToProperty encodes the Header
// field as json and stores it in a property.
func (h *Header) ToProperty() (datastore.Property, error) {
	bytes, err := json.Marshal(h)
	if err != nil {
		return datastore.Property{}, err
	}
	return datastore.MkPropertyNI(bytes), nil
}

// FromProperty extracts a Header into h from a datastore.Property.
//
// This implements PropertyConverter. FromProperty decodes the bytes stored
// in the property, which are encoded json.
func (h *Header) FromProperty(prop datastore.Property) error {
	headerBytes, ok := prop.Value().([]byte)
	if !ok {
		return errors.Reason("property for Header not stored as bytes").Err()
	}
	if err := json.Unmarshal(headerBytes, h); err != nil {
		return err
	}
	return nil
}
