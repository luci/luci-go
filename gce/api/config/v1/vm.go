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
	"bytes"
	"strings"
	"text/template"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
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
	// noindex is not respected in the tags in the model.
	return p, p.SetValue(proto.MarshalTextString(v), datastore.NoIndex)
}

// substituteZone substitutes the zone into the given zonal string template if
// it contains "{{.Zone}}", otherwise returns the string as is.
func substituteZone(s, zone string) (string, error) {
	if !strings.Contains(s, "{{.Zone}}") {
		return s, nil
	}
	t, err := template.New("tmpl").Parse(s)
	if err != nil {
		return "", err
	}
	buf := &bytes.Buffer{}
	if err := t.Execute(buf, map[string]string{
		"Zone": zone,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// SetZone sets the given zone throughout this VM. If setting one field fails,
// doesn't set any fields.
func (v *VM) SetZone(zone string) error {
	// Save the results of the substitutions.
	// Only set values if everything succeeds.
	dts := make([]string, len(v.GetDisk()))
	for i, disk := range v.GetDisk() {
		s, err := substituteZone(disk.Type, zone)
		if err != nil {
			return errors.Annotate(err, "failed to substitute %q into %q", zone, disk.Type).Err()
		}
		dts[i] = s
	}
	mt, err := substituteZone(v.GetMachineType(), zone)
	if err != nil {
		return errors.Annotate(err, "failed to substitute %q into %q", zone, v.GetMachineType()).Err()
	}

	// Substitutions succeeded. Set values.
	for i, disk := range v.GetDisk() {
		disk.Type = dts[i]
	}
	v.MachineType = mt
	v.Zone = zone
	return nil
}

// Validate validates this VM description.
// Metadata FromFile must already be converted to FromText.
func (v *VM) Validate(c *validation.Context) {
	if len(v.GetDisk()) == 0 {
		c.Errorf("at least one disk is required")
	}
	for i, disk := range v.GetDisk() {
		c.Enter("disk %d", i)
		if _, err := substituteZone(disk.Type, "zone"); err != nil {
			c.Errorf("%s", err)
		}
		c.Exit()
	}
	if v.GetMachineType() == "" {
		c.Errorf("machine type is required")
	}
	c.Enter("machine_type")
	if _, err := substituteZone(v.GetMachineType(), "zone"); err != nil {
		c.Errorf("%s", err)
	}
	c.Exit()
	for i, meta := range v.GetMetadata() {
		c.Enter("metadata %d", i)
		// Implicitly rejects FromFile.
		// FromFile must be converted to FromText before calling.
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
