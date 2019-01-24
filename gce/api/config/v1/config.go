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
	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/config/validation"
)

// Ensure Config implements datastore.PropertyConverter.
// This allows VMs blocks to be read from and written to the datastore.
var _ datastore.PropertyConverter = &Config{}

// FromProperty implements datastore.PropertyConverter.
func (cfg *Config) FromProperty(p datastore.Property) error {
	return proto.UnmarshalText(p.Value().(string), cfg)
}

// ToProperty implements datastore.PropertyConverter.
func (cfg *Config) ToProperty() (datastore.Property, error) {
	p := datastore.Property{}
	return p, p.SetValue(proto.MarshalTextString(cfg), false)
}

// Validate validates this config. Kind must already be applied.
func (cfg *Config) Validate(c *validation.Context) {
	c.Enter("attributes")
	cfg.GetAttributes().Validate(c)
	c.Exit()
	if cfg.GetPrefix() == "" {
		c.Errorf("prefix is required")
	}
	if cfg.GetSeconds() < 1 {
		// Implicitly rejects Duration.
		// TODO(smut): Support Duration.
		c.Errorf("lifetime seconds must be positive")
	}
}
