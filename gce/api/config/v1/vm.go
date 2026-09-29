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

// SetZone sets the given zone throughout this VM, substituting both "{{.Zone}}"
// templates and any previously bound zone path segments ("zones/<oldZone>/" or
// leading "<oldZone>/") in disk and machine types.
func (v *VM) SetZone(zone string) {
	oldZone := v.GetZone()
	for _, disk := range v.GetDisk() {
		if disk != nil {
			disk.Type = replaceZone(disk.Type, oldZone, zone)
		}
	}
	v.MachineType = replaceZone(v.GetMachineType(), oldZone, zone)
	v.Zone = zone
}

// replaceZone substitutes "{{.Zone}}" and any previously bound zone segment
// ("zones/<oldZone>/" or leading "<oldZone>/") in val with newZone.
func replaceZone(val, oldZone, newZone string) string {
	val = strings.ReplaceAll(val, "{{.Zone}}", newZone)
	if oldZone != "" && oldZone != newZone {
		val = strings.ReplaceAll(val, "zones/"+oldZone+"/", "zones/"+newZone+"/")
		if strings.HasPrefix(val, oldZone+"/") {
			val = newZone + strings.TrimPrefix(val, oldZone)
		}
	}
	return val
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

// validateFallbackZones validates fallback_zones constraints and regional
// subnetwork compatibility for this VM description.
func (v *VM) validateFallbackZones(c *validation.Context) {
	primaryZone := v.GetZone()
	primaryRegion := extractZoneRegion(primaryZone)
	fbs := v.GetFallbackZones()
	if len(fbs) > maxFallbackZones {
		c.Errorf("at most %d fallback zones are allowed, got %d", maxFallbackZones, len(fbs))
	}
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
		fbRegion := extractZoneRegion(fb)
		if primaryRegion == "" || fbRegion == "" || fbRegion != primaryRegion {
			c.Errorf("fallback zone %q must be in the same region as primary zone %q", fb, primaryZone)
		}
	}
	for _, nic := range v.GetNetworkInterface() {
		subRegion := extractSubnetworkRegion(nic.GetSubnetwork())
		if subRegion == "" {
			continue
		}
		if primaryZone != "" && (primaryRegion == "" || primaryRegion != subRegion) {
			c.Errorf("zone %q does not match subnetwork region %q", primaryZone, subRegion)
		}
		for _, fb := range fbs {
			if fb == "" {
				continue
			}
			if fbRegion := extractZoneRegion(fb); fbRegion == "" || fbRegion != subRegion {
				c.Errorf("fallback zone %q does not match subnetwork region %q", fb, subRegion)
			}
		}
	}
}

// extractZoneRegion returns the GCP region prefix from a zone name (for
// example, "us-central1" from "us-central1-c"), or an empty string if the zone
// does not follow the "<region>-<zone>" format.
func extractZoneRegion(zone string) string {
	idx := strings.LastIndex(zone, "-")
	if idx <= 0 || idx == len(zone)-1 {
		return ""
	}
	return zone[:idx]
}

// extractSubnetworkRegion returns the GCP region segment from a subnetwork path
// of the form "...regions/<region>/subnetworks/<name>" (for example, "us-west2"
// from "regions/us-west2/subnetworks/cloudbots-network-us-west2"), or an empty
// string if no regional segment is present.
func extractSubnetworkRegion(subnetwork string) string {
	parts := strings.Split(subnetwork, "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "regions" && parts[i+2] == "subnetworks" && parts[i+1] != "" {
			return parts[i+1]
		}
	}
	return ""
}
