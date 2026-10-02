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

// zoneTemplate is the placeholder token substituted by SetZone in zone-scoped
// VM resource fields such as machine_type and disk.type.
const zoneTemplate = "{{.Zone}}"

// SetZone sets the given zone throughout this VM, substituting "{{.Zone}}"
// templates in disk and machine types and clearing fallback_zones since the
// zone templates have been resolved.
func (v *VM) SetZone(zone string) {
	for _, disk := range v.GetDisk() {
		if disk != nil {
			disk.Type = strings.ReplaceAll(disk.Type, zoneTemplate, zone)
		}
	}
	v.MachineType = strings.ReplaceAll(v.GetMachineType(), zoneTemplate, zone)
	v.Zone = zone
	v.FallbackZones = nil
}

// maxFallbackZones is the maximum number of fallback zones allowed per VM
// configuration to bound per-task latency within Cloud Tasks execution limits.
const maxFallbackZones = 3

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
	v.validateFallbackZones(c)
}

// validateFallbackZones validates fallback_zones constraints and "{{.Zone}}"
// template requirements for this VM description.
//
// Note: Fallback zones cannot use different regions because regional
// configurations such as network_interface.subnetwork are defined per region
// and dynamic selection of regional networks is not supported yet.
func (v *VM) validateFallbackZones(c *validation.Context) {
	fbs := v.GetFallbackZones()
	if len(fbs) == 0 {
		return
	}
	if len(fbs) > maxFallbackZones {
		c.Errorf("at most %d fallback zones are allowed, got %d", maxFallbackZones, len(fbs))
	}
	primaryZone := v.GetZone()
	seen := make(map[string]bool, len(fbs))
	for i, fb := range fbs {
		if fb == "" {
			c.Errorf("fallback zone %d cannot be empty", i)
			continue
		}
		if fb == primaryZone {
			c.Errorf("fallback zone %q cannot be the same as primary zone %q", fb, primaryZone)
		}
		if seen[fb] {
			c.Errorf("duplicate fallback zone %q", fb)
		}
		seen[fb] = true
	}
	if mt := v.GetMachineType(); mt != "" && !strings.Contains(mt, zoneTemplate) {
		c.Errorf("machine type %q must contain %s template when fallback_zones is set", mt, zoneTemplate)
	}
	for i, d := range v.GetDisk() {
		if dt := d.GetType(); dt != "" && !strings.Contains(dt, zoneTemplate) {
			c.Errorf("disk %d type %q must contain %s template when fallback_zones is set", i, dt, zoneTemplate)
		}
	}
}
