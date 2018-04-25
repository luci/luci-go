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
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"go.chromium.org/luci/common/data/strpair"
)

// PoolDescriptor describes the attributes of a pool in "tag:value" format.
// This is defined as Swarming Dimensions + Hostname.
type PoolDescriptor []string

// SwarmingHostnameKey is a magic PoolDescriptor tag containing the Swarming Hostname.
const SwarmingHostnameKey = "swarming_hostname"

func NewPoolDescriptor(host string, dimensions []string) PoolDescriptor {
	desc := make([]string, 0, len(dimensions)+1)
	desc = append(desc, fmt.Sprintf("%s:%s", SwarmingHostnameKey, host))
	desc = append(desc, dimensions...)
	sort.Strings(desc)
	return desc
}

// PoolKey returns they key for a BotPool with the specified PoolDescriptor.
// The key is a SHA1 hash in hex form.
// descriptor must be pre-sorted (I.E. Created with NewPoolDescriptor)
func (d PoolDescriptor) PoolKey() string {
	s, err := json.Marshal(d)
	if err != nil {
		// This should never happen for a slice of strings.
		panic(err)
	}
	return fmt.Sprintf("%x", sha1.Sum(s))
}

// Host returns the swarming_hostname item of the PoolDescriptor.
func (d PoolDescriptor) Host() string {
	return strpair.ParseMap(d).Get(SwarmingHostnameKey)
}

// Dimensions returns a copy of the PoolDescriptor without the swarming_hostname item.
func (d PoolDescriptor) Dimensions() []string {
	m := strpair.ParseMap(d)
	m.Del(SwarmingHostnameKey)
	return m.Format()
}

// BotPoolKind is the name of the datastore entity kind of BotPool.
const BotPoolKind = "BotPool"

// BotPool is the execution pool information associated with a
// set of Swarming bots.  This information is periodically refreshed from Swarming.
type BotPool struct {
	// PoolKey is an identifier for the BotPool.
	// PoolKey should have a 1:1 relationship with the Descriptor.
	// This field should always be constructed with BotPoolKey().
	PoolKey string `gae:"$id"`

	// Descriptor uniquely describes this BotPool.
	// It is defined as Swarming Dimensions + Hostname
	Descriptor PoolDescriptor

	// Bots is a slice of bots in the pool, along with its status.
	Bots Bots

	// LastUpdate is when this entity was last updated.
	LastUpdate time.Time
}

// Bots is a slice of Bot.
type Bots []Bot

// Bot represents a single job executor.
type Bot struct {
	// Name is an identifier for the Bot.  This is usually a short hostname.
	Name string
	// Status is the current status of the Bot.
	Status BotStatus
	// LastSeen denotes when the Bot was last seen.
	LastSeen time.Time
}
