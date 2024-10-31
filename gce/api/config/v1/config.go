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
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"
)

// Ensure Config implements datastore.PropertyConverter.
// This allows VMs blocks to be read from and written to the datastore.
var _ datastore.PropertyConverter = &Config{}

// FromProperty implements datastore.PropertyConverter.
func (cfg *Config) FromProperty(p datastore.Property) error {
	if p.Value() == nil {
		cfg = &Config{}
		return nil
	}
	return proto.Unmarshal(p.Value().([]byte), cfg)
}

// ToProperty implements datastore.PropertyConverter.
func (cfg *Config) ToProperty() (datastore.Property, error) {
	p := datastore.Property{}
	bytes, err := proto.Marshal(cfg)
	if err != nil {
		return datastore.Property{}, err
	}
	// noindex is not respected in the tags in the model.
	return p, p.SetValue(bytes, datastore.NoIndex)
}

// ComputeAmount returns the amount to use given the proposed amount and time.
// Assumes this config has been validated.
func (cfg *Config) ComputeAmount(proposed int32, now time.Time) (int32, error) {
	return cfg.GetAmount().getAmount(proposed, now)
}

// Validate validates this config.
func (cfg *Config) Validate(c *validation.Context, metadataFromFileResolved bool) {
	c.Enter("amount")
	cfg.GetAmount().Validate(c)
	c.Exit()
	c.Enter("attributes")
	cfg.GetAttributes().Validate(c, metadataFromFileResolved)
	c.Exit()
	if cfg.GetCurrentAmount() != 0 {
		c.Errorf("current amount must not be specified")
	}
	c.Enter("lifetime")
	cfg.GetLifetime().Validate(c)
	switch n, err := cfg.Lifetime.ToSeconds(); {
	case err != nil:
		c.Errorf("%s", err)
	case n == 0:
		c.Errorf("duration or seconds is required")
	}
	c.Exit()
	if cfg.GetCurrentAmount() != 0 {
		c.Errorf("current amount must not be specified")
	}
	if cfg.GetPrefix() == "" {
		c.Errorf("prefix is required")
	}
	if cfg.GetRevision() != "" {
		c.Errorf("revision must not be specified")
	}
	c.Enter("timeout")
	cfg.GetTimeout().Validate(c)
	c.Exit()
}
