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

package projects

import (
	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/config/validation"
)

// Ensure Config implements datastore.PropertyConverter.
// This allows projects to be read from and written to the datastore.
var _ datastore.PropertyConverter = &Config{}

// FromProperty implements datastore.PropertyConverter.
func (cfg *Config) FromProperty(p datastore.Property) error {
	return proto.UnmarshalText(p.Value().(string), cfg)
}

// ToProperty implements datastore.PropertyConverter.
func (cfg *Config) ToProperty() (datastore.Property, error) {
	p := datastore.Property{}
	// noindex is not respected in the tags in the model.
	return p, p.SetValue(proto.MarshalTextString(cfg), datastore.NoIndex)
}

// Validate validates this config.
func (cfg *Config) Validate(c *validation.Context) {
	if cfg.GetProject() == "" {
		c.Errorf("project is required")
	}
	if cfg.GetRevision() != "" {
		c.Errorf("revision must not be specified")
	}
}
