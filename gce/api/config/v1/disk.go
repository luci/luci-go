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

// GetImageBase returns the base image name for this validated disk.
func (d *Disk) GetImageBase() string {
	return d.GetImage()[strings.LastIndex(d.GetImage(), "/")+1:]
}

// Validate validates this disk.
func (d *Disk) Validate(c *validation.Context) {
	if !isValidImage(d.GetImage()) {
		c.Errorf("image must match projects/<project>/global/images/<image> or global/images/<image>")
	}
}
