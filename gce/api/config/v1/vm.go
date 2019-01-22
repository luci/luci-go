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
	"strings"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/config/validation"
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

// Validate validates this VM description.
func (v *VM) Validate(c *validation.Context) {
	if len(v.GetDisk()) == 0 {
		c.Errorf("at least one disk is required")
	}
	if v.GetMachineType() == "" {
		c.Errorf("machine type is required")
	}
	for i, meta := range v.GetMetadata() {
		c.Enter("metadata %d", i)
		// Implicitly rejects FromFile.
		// TODO(smut): Support FromFile.
		if !strings.Contains(meta.GetFromText(), ":") {
			c.Errorf("metadata from text must be in key:value form")
		}
		c.Exit()
	}
	if len(v.GetNetworkInterface()) == 0 {
		c.Errorf("at least one network interface is required")
	}
	if v.GetProject() == "" {
		c.Errorf("project is required")
	}
	if v.GetZone() == "" {
		c.Errorf("zone is required")
	}
}
