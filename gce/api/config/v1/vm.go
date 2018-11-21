// Copyright 2018 The LUCI Authors.
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
	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
)

// Ensure VM implements datastore.PropertyConverter.
// This allows VMs to be read from and written to the datastore.
var _ datastore.PropertyConverter = &VM{}

// FromProperty implements datastore.PropertyConverter.
func (v *VM) FromProperty(p datastore.Property) error {
	return proto.UnmarshalText(p.Value().(string), v)
}

// ToProperty implements datastore.PropertyConverter.
func (v *VM) ToProperty() (datastore.Property, error) {
	p := datastore.Property{}
	return p, p.SetValue(proto.MarshalTextString(v), false)
}
