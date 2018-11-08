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

package model

import (
	"context"
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/gce/api/config/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestVMs(t *testing.T) {
	t.Parallel()

	Convey("datastore", t, func() {
		c := memory.Use(context.Background())
		v := &VMs{ID: "test"}
		err := datastore.Get(c, v)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		err = datastore.Put(c, &VMs{
			ID: "test",
			Config: config.Block{
				Attributes: &config.VM{
					Project: "project",
				},
				Prefix: "prefix",
			},
		})
		So(err, ShouldBeNil)

		err = datastore.Get(c, v)
		So(err, ShouldBeNil)
		So(v, ShouldResemble, &VMs{
			ID: "test",
			Config: config.Block{
				Attributes: &config.VM{
					Project: "project",
				},
				Prefix: "prefix",
			},
		})
	})
}
