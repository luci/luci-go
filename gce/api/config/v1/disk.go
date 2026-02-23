// Copyright 2019 The LUCI Authors.
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

	"go.chromium.org/luci/config/validation"
)

// isValidImage returns whether or not the given string is a valid image name.
// Image names must have the form projects/<project>/global/images/<image> or
// global/images/<image>.
func isValidImage(s string) bool {
	switch parts := strings.Split(s, "/"); len(parts) {
	case 3:
		return parts[0] == "global" && parts[1] == "images"
	case 5:
		return parts[0] == "projects" && parts[2] == "global" && parts[3] == "images"
	}
	return false
}

// IsPersistentDisk returns whether or not the given string is a persistent
// disk type.
func (d *Disk) IsPersistentDisk() bool {
	return strings.HasSuffix(d.Type, "/pd-standard") || strings.HasSuffix(d.Type, "/pd-ssd")
}

// IsScratchDisk returns whether or not the given string is a scratch disk
// type.
func (d *Disk) IsScratchDisk() bool {
	return strings.HasSuffix(d.Type, "/local-ssd")
}

// isHyperdisk returns whether or not the given disk type is a Hyperdisk type.
func (d *Disk) isHyperdisk() bool {
	return strings.Contains(d.Type, "hyperdisk")
}

// isValidDiskType returns whether or not the given string is a valid disk
// type. Disk types have the form zones/<zone>/diskTypes/<type>.
func isValidDiskType(s string) bool {
	// Empty disk type implies the default.
	if s == "" {
		return true
	}
	parts := strings.Split(s, "/")
	return len(parts) == 4 && parts[0] == "zones" && parts[2] == "diskTypes"
}

// GetImageBase returns the base image name for this validated disk.
func (d *Disk) GetImageBase() string {
	return d.GetImage()[strings.LastIndex(d.GetImage(), "/")+1:]
}

// Validate validates this disk.
//
//	The set of valid configurations is:
//	+-------------+-------+-----------+-----------------------------+
//	| Type        | Image | Interface | Provisioned IOPS/Throughput |
//	+-------------+-------+-----------+-----------------------------+
//	| local-ssd   | No    | *         | No                          |
//	| pd-ssd      | Yes   | SCSI      | No                          |
//	| pd-standard | Yes   | SCSI      | No                          |
//	| hyperdisk-* | Yes   | NVME (recommended)      | Yes (Optional)              |
//	+-------------+-------+-----------+-----------------------------+
func (d *Disk) Validate(c *validation.Context) {
	if !isValidDiskType(d.Type) {
		c.Errorf("disk type must match zones/<zone>/diskTypes/<type>")
	}

	if d.IsPersistentDisk() {
		// PD disks must use SCSI.
		if d.GetInterface() != DiskInterface_SCSI {
			c.Errorf("persistent disk must use SCSI")
		}
		if !isValidImage(d.GetImage()) {
			c.Errorf("image must match projects/<project>/global/images/<image> or global/images/<image>")
		}
	}
	if d.IsScratchDisk() && isValidImage(d.GetImage()) {
		c.Errorf("local ssd cannot use an image")
	}

	if d.isHyperdisk() {
		if d.ProvisionedIops < 0 {
			c.Errorf("provisioned_iops must be non-negative")
		}
		if d.ProvisionedThroughput < 0 {
			c.Errorf("provisioned_throughput must be non-negative")
		}
	} else {
		// Provisioning fields only for Hyperdisk.
		if d.ProvisionedIops > 0 {
			c.Errorf("provisioned_iops can only be set for Hyperdisk types")
		}
		if d.ProvisionedThroughput > 0 {
			c.Errorf("provisioned_throughput can only be set for Hyperdisk types")
		}
	}
}
