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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"go.chromium.org/luci/common/data/strpair"

	"go.chromium.org/luci/milo/internal/model/milostatus"
)

// PoolDescriptor describes the attributes of a pool in "tag:value" format.
// This is defined as Swarming Dimensions + Hostname.
type PoolDescriptor []string

// SwarmingHostnameKey is a magic PoolDescriptor tag containing the Swarming Hostname.
const SwarmingHostnameKey = "swarming_hostname"

// NewPoolDescriptor creates a new PoolDescriptor given a swarming hostname
// and a slice of dimensions.  The dimensions are expected to be in "key:value" format,
// where the key is the tag name.  This is the same format returned by the Swarming API.
func NewPoolDescriptor(host string, dimensions []string) PoolDescriptor {
	desc := make([]string, 0, len(dimensions)+1)
	desc = append(desc, fmt.Sprintf("%s:%s", SwarmingHostnameKey, host))
	desc = append(desc, dimensions...)
	sort.Strings(desc)
	return desc
}

// PoolID returns they ID for a BotPool with the specified PoolDescriptor.
// The ID is a SHA256 hash in hex form.
// descriptor must be pre-sorted (I.E. Created with NewPoolDescriptor).
func (d PoolDescriptor) PoolID() string {
	s, err := json.Marshal(d)
	if err != nil {
		// This should never happen for a slice of strings.
		panic(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(s))
}

// Host returns the swarming_hostname item of the PoolDescriptor.
func (d PoolDescriptor) Host() string {
	return strpair.ParseMap(d).Get(SwarmingHostnameKey)
}

// Dimensions returns a copy of the PoolDescriptor without the swarming_hostname item
// in strpair.Map format.
func (d PoolDescriptor) Dimensions() strpair.Map {
	m := strpair.ParseMap(d)
	m.Del(SwarmingHostnameKey)
	return m
}

// BotPoolKind is the name of the datastore entity kind of BotPool.
const BotPoolKind = "BotPool"

// BotPool is the execution pool information associated with a
// set of Swarming bots.  This information is periodically refreshed from Swarming.
type BotPool struct {
	// PoolID is an identifier for the BotPool.
	// PoolID must match the Descriptor.
	// This field should always be constructed with PoolID().
	PoolID string `gae:"$id"`

	// Descriptor uniquely describes this BotPool.
	// It is defined as Swarming Dimensions + Hostname.
	Descriptor PoolDescriptor

	// Bots is a slice of bots in the pool, along with their statuses.
	Bots []Bot `gae:",noindex"`

	// LastUpdate is when this entity was last updated.
	LastUpdate time.Time
}

// Bot represents a single job executor.
type Bot struct {
	// Name is an identifier for the Bot.  This is usually a short hostname.
	Name string
	// URL is a link to a bot page, if available.
	URL string
	// Status is the current status of the Bot.
	Status milostatus.BotStatus
	// LastSeen denotes when the Bot was last seen.
	LastSeen time.Time
}
