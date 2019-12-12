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
	"go.chromium.org/luci/config/validation"
)

// isSpecified returns whether or not this adjustable is specified.
func (a *Adjustable) isSpecified() bool {
	return a.GetMin() != 0 || a.GetMax() != 0 || len(a.GetGroup()) > 0
}

// Validate validates this adjustable.
func (a *Adjustable) Validate(c *validation.Context) {
	if a.GetMin() < 0 {
		c.Errorf("minimum amount must be non-negative")
	}
	if a.GetMax() < 0 {
		c.Errorf("maximum amount must be non-negative")
	}
	if a.GetMin() > a.GetMax() {
		c.Errorf("minimum amount must be less than maximum amount")
	}
}
