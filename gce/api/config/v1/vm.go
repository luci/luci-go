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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"
)

// Ensure VM implements datastore.PropertyConverter.
// This allows VMs to be read from and written to the datastore.
var _ datastore.PropertyConverter = &VM{}

// FromProperty implements datastore.PropertyConverter.
func (v *VM) FromProperty(p datastore.Property) error {
	if p.Value() == nil {
		v = &VM{}
		return nil
	}
	return proto.Unmarshal(p.Value().([]byte), v)
}

// ToProperty implements datastore.PropertyConverter.
func (v *VM) ToProperty() (datastore.Property, error) {
	p := datastore.Property{}
	bytes, err := proto.Marshal(v)
	if err != nil {
		return datastore.Property{}, err
	}
	// noindex is not respected in the tags in the model.
	return p, p.SetValue(bytes, datastore.NoIndex)
}

// SetZone sets the given zone throughout this VM.
func (v *VM) SetZone(zone string) {
	for _, disk := range v.GetDisk() {
		disk.Type = strings.Replace(disk.Type, "{{.Zone}}", zone, -1)
	}
	v.MachineType = strings.Replace(v.GetMachineType(), "{{.Zone}}", zone, -1)
	v.Zone = zone
}

// Validate validates this VM description.
func (v *VM) Validate(c *validation.Context, metadataFromFileResolved bool) {
	if len(v.GetDisk()) == 0 {
		c.Errorf("at least one disk is required")
	}
	for i, d := range v.GetDisk() {
		c.Enter("disk %d", i)
		d.Validate(c)
		c.Exit()
	}
	if v.GetMachineType() == "" {
		c.Errorf("machine type is required")
	}
	for i, meta := range v.GetMetadata() {
		c.Enter("metadata %d", i)
		if fromFile := meta.GetFromFile(); !metadataFromFileResolved && fromFile != "" {
			if !strings.Contains(fromFile, ":") {
				c.Errorf("metadata from file must be in key:value form")
			}
		} else {
			if !strings.Contains(meta.GetFromText(), ":") {
				c.Errorf("metadata from text must be in key:value form")
			}
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
