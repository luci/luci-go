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
//	+-------------+-------+-----------+
//	| Type        | Image | Interface |
//	+-------------+-------+-----------+
//	| local-ssd   | No    | *         |
//	| pd-ssd      | Yes   | SCSI      |
//	| pd-standard | Yes   | SCSI      |
//	+-------------+-------+-----------+
func (d *Disk) Validate(c *validation.Context) {
	if !isValidDiskType(d.Type) {
		c.Errorf("disk type must match zones/<zone>/diskTypes/<type>")
	}
	if d.IsPersistentDisk() && d.GetInterface() != DiskInterface_SCSI {
		c.Errorf("persistent disk must use SCSI")
	}
	if d.IsPersistentDisk() && !isValidImage(d.GetImage()) {
		c.Errorf("image must match projects/<project>/global/images/<image> or global/images/<image>")
	}
	if d.IsScratchDisk() && isValidImage(d.GetImage()) {
		c.Errorf("local ssd cannot use an image")
	}
}
