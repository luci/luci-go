// Copyright 2023 The LUCI Authors.
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
	"strings"
	"time"

	"go.chromium.org/luci/gae/service/datastore"
)

// BotInfo contains the latest information about a bot.
type BotInfo struct {
	// ID is always `info`.
	ID string `gae:"$id,info"`
	// Parent is the parent BotRootKey(...) with bot ID.
	Parent *datastore.Key `gae:"$parent"`

	// Composite encodes the state of the bot.
	//
	// One of:
	//  NOT_IN_MAINTENANCE = 1 << 9  # 512
	//  IN_MAINTENANCE = 1 << 8  # 256
	// One of:
	//  ALIVE = 1 << 7  # 128
	//  DEAD = 1 << 6  # 64
	// One of:
	//  HEALTHY = 1 << 3  # 8
	//  QUARANTINED = 1 << 2  # 4
	// One of:
	//  IDLE = 1<<1 # 2
	//  BUSY = 1<<0 # 1
	Composite []int64 `gae:"composite"`

	// State is a state dict reported by the bot in JSON encoding.
	State []byte `gae:"state,noindex"`

	// Dimensions is a list of dimensions reported by the bot as "k:v" pairs.
	Dimensions []string `gae:"dimensions_flat"`

	// LastSeen is when this bot pinged Swarming last time (if ever).
	LastSeen time.Time `gae:"last_seen_ts,noindex"`

	// Quarantined is true if the bot is in quarantine.
	Quarantined bool `gae:"quarantined,noindex"`

	_extra datastore.PropertyMap `gae:"-,extra"`
}

// IsDead is true if this bot is considered dead.
func (b *BotInfo) IsDead() bool {
	for _, v := range b.Composite {
		switch v {
		case 128:
			return false
		case 64:
			return true
		}
	}
	return false
}

// IsInMaintenance is true if this bot is in maintenance.
func (b *BotInfo) IsInMaintenance() bool {
	for _, v := range b.Composite {
		switch v {
		case 512:
			return false
		case 256:
			return true
		}
	}
	return false
}

// GetStatus returns the bot status.
func (b *BotInfo) GetStatus() string {
	for _, v := range b.Composite {
		switch v {
		case 256:
			return "maintenance"
		case 4:
			return "quarantined"
		case 64:
			return "dead"
		case 1:
			return "running"
		}
	}
	return "ready"
}

// DimenionsByKey returns a list of dimension values with the given key.
func (b *BotInfo) DimenionsByKey(k string) (values []string) {
	pfx := k + ":"
	for _, kv := range b.Dimensions {
		if val, ok := strings.CutPrefix(kv, pfx); ok {
			values = append(values, val)
		}
	}
	return values
}

// BotInfoQuery prepares a query that fetches BotInfo entities.
func BotInfoQuery() *datastore.Query {
	return datastore.NewQuery("BotInfo")
}

// BotRootKey is a root key of an entity group with info about a bot.
func BotRootKey(ctx context.Context, botID string) *datastore.Key {
	return datastore.NewKey(ctx, "BotRoot", botID, 0, nil)
}

// BotDimensions is a map with bot dimensions as `key => [values]`.
//
// This type represents bot dimensions in the datastore as a JSON-encoded
// unindexed blob. There's an alternative "flat" indexed representation as a
// list of `key:value` pairs. It is used in BotInfo.Dimensions property.
type BotDimensions map[string][]string

// ToProperty stores the value as a JSON-blob property.
func (p *BotDimensions) ToProperty() (datastore.Property, error) {
	return ToJSONProperty(p)
}

// FromProperty loads a JSON-blob property.
func (p *BotDimensions) FromProperty(prop datastore.Property) error {
	return FromJSONProperty(prop, p)
}
