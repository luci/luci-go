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
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"
)

// Validate validates this VMs block given the allowable kinds the block may reference.
func (b *Block) Validate(c *validation.Context, kinds stringset.Set) {
	if b.Kind != "" && !kinds.Has(b.Kind) {
		c.Errorf("unknown kind %q", b.Kind)
	}
}
