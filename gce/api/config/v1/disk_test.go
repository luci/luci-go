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
	"context"
	"testing"

	"go.chromium.org/luci/config/validation"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDisk(t *testing.T) {
	t.Parallel()

	ftt.Run("Disk", t, func(t *ftt.Test) {
		t.Run("isValidImage", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				assert.Loosely(t, isValidImage("image"), should.BeFalse)
				assert.Loosely(t, isValidImage("global/image"), should.BeFalse)
				assert.Loosely(t, isValidImage("projects/image"), should.BeFalse)
				assert.Loosely(t, isValidImage("projects/global/image"), should.BeFalse)
				assert.Loosely(t, isValidImage("projects/project/region/image"), should.BeFalse)
				assert.Loosely(t, isValidImage("projects/project/region/images/image"), should.BeFalse)
			})

			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, isValidImage("global/images/image"), should.BeTrue)
				assert.Loosely(t, isValidImage("projects/project/global/images/image"), should.BeTrue)
			})
		})

		t.Run("GetImageBase", func(t *ftt.Test) {
			t.Run("short", func(t *ftt.Test) {
				d := &Disk{
					Image: "global/images/image",
				}
				assert.Loosely(t, d.GetImageBase(), should.Equal("image"))
			})

			t.Run("long", func(t *ftt.Test) {
				d := &Disk{
					Image: "projects/project/global/images/image",
				}
				assert.Loosely(t, d.GetImageBase(), should.Equal("image"))
			})
		})

		t.Run("Validate", func(t *ftt.Test) {
			c := &validation.Context{Context: context.Background()}
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("Persistent + NVMe", func(t *ftt.Test) {
					d := &Disk{
						Type:      "zones/zone/diskTypes/pd-standard",
						Interface: DiskInterface_NVME,
						Image:     "",
					}
					d.Validate(c)
					err := c.Finalize().(*validation.Error).Errors
					assert.Loosely(t, err, convey.Adapt(ShouldContainErr)("persistent disk must use SCSI"))
				})
				t.Run("Persistent + No Image", func(t *ftt.Test) {
					d := &Disk{
						Type:      "zones/zone/diskTypes/pd-ssd",
						Interface: DiskInterface_SCSI,
						Image:     "",
					}
					d.Validate(c)
					err := c.Finalize().(*validation.Error).Errors
					assert.Loosely(t, err, convey.Adapt(ShouldContainErr)("image must match"))
				})
				t.Run("Scratch + Image", func(t *ftt.Test) {
					d := &Disk{
						Type:      "zones/zone/diskTypes/local-ssd",
						Interface: DiskInterface_NVME,
						Image:     "global/images/image",
					}
					d.Validate(c)
					err := c.Finalize().(*validation.Error).Errors
					assert.Loosely(t, err, convey.Adapt(ShouldContainErr)("local ssd cannot use an image"))
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				t.Run("Persistent + SCSI + Image", func(t *ftt.Test) {
					d := &Disk{
						Type:      "zones/zone/diskTypes/pd-standard",
						Interface: DiskInterface_SCSI,
						Image:     "global/images/image",
					}
					d.Validate(c)
					assert.Loosely(t, c.Finalize(), should.BeNil)
				})
				t.Run("Scratch + No Image", func(t *ftt.Test) {
					d := &Disk{
						Type:      "zones/zone/diskTypes/local-ssd",
						Interface: DiskInterface_NVME,
						Image:     "",
					}
					d.Validate(c)
					assert.Loosely(t, c.Finalize(), should.BeNil)
				})
			})
		})
	})
}
