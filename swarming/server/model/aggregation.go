// Copyright 2024 The LUCI Authors.
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
	"time"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/model/internalmodelpb"
)

// BotsDimensionsAggregation stores all observed bot dimensions per pool.
//
// It is updated by scan.BotsDimensionsAggregator.
type BotsDimensionsAggregation struct {
	_ datastore.PropertyMap `gae:"-,extra"`

	// Key is BotsDimensionsAggregationKey.
	Key *datastore.Key `gae:"$key"`
	// LastUpdate is when this entity changed the last time.
	LastUpdate time.Time `gae:",noindex"`

	// Dimensions is actual aggregated dimensions map.
	//
	// Stored internally as zstd-compressed proto (since the datastore library
	// compressed protos by default if they are larger than some threshold).
	Dimensions *internalmodelpb.AggregatedDimensions
}

// BotsDimensionsAggregationInfo contains info about BotsDimensionsAggregation.
//
// It is tiny in comparison. Its LastUpdate field can be used to quickly check
// if the aggregated dimensions set has changed and needs to be reloaded. Both
// entities are always updated in the same transaction.
//
// It is updated by scan.BotsDimensionsAggregator.
type BotsDimensionsAggregationInfo struct {
	_ datastore.PropertyMap `gae:"-,extra"`

	// Key is BotsDimensionsAggregationInfoKey.
	Key *datastore.Key `gae:"$key"`
	// LastUpdate is when BotsDimensionsAggregation changed the last time.
	LastUpdate time.Time `gae:",noindex"`
}

// BotsDimensionsAggregationKey is BotsDimensionsAggregation entity key.
func BotsDimensionsAggregationKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "BotsDimensionsAggregation", "", 1, nil)
}

// BotsDimensionsAggregationInfoKey is BotsDimensionsAggregationInfo entity key.
func BotsDimensionsAggregationInfoKey(ctx context.Context) *datastore.Key {
	return datastore.NewKey(ctx, "BotsDimensionsAggregationInfo", "", 1, BotsDimensionsAggregationKey(ctx))
}
