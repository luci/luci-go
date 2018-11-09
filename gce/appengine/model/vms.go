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
	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/gce/api/config/v1"
)

// VMs is a root entity representing a configured block of VMs.
type VMs struct {
	// _extra is where unknown properties are put into memory.
	// Extra properties are not written to the datastore.
	_extra datastore.PropertyMap `gae:"-,extra"`
	// _kind is the entity's kind in the datastore.
	_kind string `gae:"$kind,VMs"`
	// ID is the unique identifier for this VMs block.
	ID string `gae:"$id"`
	// Config is the config.Block representation of this entity.
	Config config.Block `gae:"config"`
}
