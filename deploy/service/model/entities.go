// Copyright 2022 The LUCI Authors.
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

// Package model contains Datastore entities definition.
package model

import (
	"sort"
	"time"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/gae/service/datastore"
)

// Asset represents a Cloud resource (or a bunch of resources) actuated as
// a single unit.
type Asset struct {
	_kind  string                `gae:"$kind,Asset"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	// ID is the unique ID of this asset matching Asset.Id.
	ID string `gae:"$id"`
	// Asset contains all details about the asset.
	Asset *modelpb.Asset
}

// Actuation is an inflight or finished actuation of some deployment.
type Actuation struct {
	_kind  string                `gae:"$kind,Actuation"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	// ID is the unique ID of the actuation matching Actuation.Id.
	ID string `gae:"$id"`
	// Actuation contains all details about the actuation.
	Actuation *modelpb.Actuation
	// Decisions is what the backend decided to do with the actuated assets.
	Decisions *modelpb.ActuationDecisions

	// State matches Actuation.State, exposed for indexing.
	State modelpb.Actuation_State
	// Created matches Actuation.Created, exposed for indexing.
	Created time.Time
	// Expiry matches Actuation.Expiry, exposed for indexing.
	Expiry time.Time
}

// AssetIDs is a sorted list of asset IDs actuated by this actuation.
func (a *Actuation) AssetIDs() []string {
	ids := make([]string, 0, len(a.Decisions.GetDecisions()))
	for id := range a.Decisions.GetDecisions() {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
