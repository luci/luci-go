// Copyright 2017 The LUCI Authors.
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
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// Rack represents a rack.
type Rack struct {
	config.RackConfig
	DatacenterId int64
	Id           int64
}

// RacksTable represents the table of racks in the database.
type RacksTable struct {
	// datacenters is a map of datacenter name to ID in the database.
	datacenters map[string]int64
	// current is the slice of racks in the database.
	current []*Rack

	// additions is a slice of racks pending addition to the database.
	additions []*Rack
	// removals is a slice of racks pending removal from the database.
	removals []string
	// updates is a slice of racks pending update in the database.
	updates []*Rack
}

// fetch fetches the racks from the database.
func (r *RacksTable) fetch(c context.Context) error {
	db := database.Get(c)
	rows, err := db.QueryContext(c, "SELECT id, name, description, datacenter_id FROM racks")
	if err != nil {
		return errors.Annotate(err, "failed to select racks").Err()
	}
	defer rows.Close()
	for rows.Next() {
		rack := &Rack{}
		if err := rows.Scan(&rack.Id, &rack.Name, &rack.Description, &rack.DatacenterId); err != nil {
			return errors.Annotate(err, "failed to scan rack").Err()
		}
		r.current = append(r.current, rack)
	}
	return nil
}

// computeChanges computes the changes that need to be made to the racks in the database.
func (r *RacksTable) computeChanges(c context.Context, datacenters []*config.DatacenterConfig) error {
	cfgs := make(map[string]*Rack, len(datacenters))
	for _, dc := range datacenters {
		for _, cfg := range dc.Rack {
			id, ok := r.datacenters[dc.Name]
			if !ok {
				return errors.Reason("failed to determine datacenter ID for rack: %s: unknown datacenter: %s", cfg.Name, dc.Name).Err()
			}
			cfgs[cfg.Name] = &Rack{
				RackConfig: config.RackConfig{
					Name:        cfg.Name,
					Description: cfg.Description,
				},
				DatacenterId: id,
			}
		}
	}

	for _, rack := range r.current {
		if cfg, ok := cfgs[rack.Name]; ok {
			// Rack found in the config.
			if rack.Description != cfg.Description || rack.DatacenterId != cfg.DatacenterId {
				// Rack doesn't match the config.
				r.updates = append(r.updates, cfg)
				cfg.Id = rack.Id
			}
			// Record that the rack config has been seen.
			delete(cfgs, cfg.Name)
		} else {
			// Rack not found in the config.
			r.removals = append(r.removals, rack.Name)
		}
	}

	// Racks remaining in the map are present in the config but not the database.
	// Iterate deterministically over the slice to determine which racks need to be added.
	for _, dc := range datacenters {
		for _, cfg := range dc.Rack {
			if rack, ok := cfgs[cfg.Name]; ok {
				r.additions = append(r.additions, rack)
			}
		}
	}
	return nil
}

// add adds all racks pending addition to the database, clearing pending additions.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (r *RacksTable) add(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(r.additions) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, "INSERT INTO racks (name, description, datacenter_id) VALUES (?, ?, ?)")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Add each rack to the database, and update the slice of racks with each addition.
	for len(r.additions) > 0 {
		rack := r.additions[0]
		result, err := stmt.ExecContext(c, rack.Name, rack.Description, rack.DatacenterId)
		if err != nil {
			return errors.Annotate(err, "failed to add rack: %s", rack.Name).Err()
		}
		r.current = append(r.current, rack)
		r.additions = r.additions[1:]
		logging.Infof(c, "Added rack: %s", rack.Name)
		rack.Id, err = result.LastInsertId()
		if err != nil {
			return errors.Annotate(err, "failed to get rack ID: %s", rack.Name).Err()
		}
	}
	return nil
}

// remove removes all racks pending removal from the database, clearing pending removals.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (r *RacksTable) remove(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(r.removals) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, "DELETE FROM racks WHERE name = ?")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Remove each rack from the database. It's more efficient to update the slice of
	// racks once at the end rather than for each removal, so use a defer.
	removed := stringset.New(len(r.removals))
	defer func() {
		var racks []*Rack
		for _, rack := range r.current {
			if !removed.Has(rack.Name) {
				racks = append(racks, rack)
			}
		}
		r.current = racks
	}()
	for len(r.removals) > 0 {
		rack := r.removals[0]
		if _, err := stmt.ExecContext(c, rack); err != nil {
			// Defer ensures the slice of racks is updated even if we exit early.
			return errors.Annotate(err, "failed to remove rack: %s", rack).Err()
		}
		removed.Add(rack)
		r.removals = r.removals[1:]
		logging.Infof(c, "Removed rack: %s", rack)
	}
	return nil
}

// update updates all racks pending update in the database, clearing pending updates.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (r *RacksTable) update(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(r.updates) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, "UPDATE racks SET description = ?, datacenter_id = ? WHERE id = ?")
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Update each rack in the database. It's more efficient to update the slice of
	// racks once at the end rather than for each update, so use a defer.
	updated := make(map[string]*Rack, len(r.updates))
	defer func() {
		for _, rack := range r.current {
			if _, ok := updated[rack.Name]; ok {
				rack.Description = updated[rack.Name].Description
				rack.DatacenterId = updated[rack.Name].DatacenterId
			}
		}
	}()
	for len(r.updates) > 0 {
		rack := r.updates[0]
		if _, err := stmt.ExecContext(c, rack.Description, rack.DatacenterId, rack.Id); err != nil {
			return errors.Annotate(err, "failed to update rack: %s", rack.Name).Err()
		}
		updated[rack.Name] = rack
		r.updates = r.updates[1:]
		logging.Infof(c, "Updated rack: %s", rack.Name)
	}
	return nil
}

// ids returns a map of rack names to IDs.
func (r *RacksTable) ids(c context.Context) map[string]int64 {
	racks := make(map[string]int64, len(r.current))
	for _, rack := range r.current {
		racks[rack.Name] = rack.Id
	}
	return racks
}

// EnsureRacks ensures the database contains exactly the given racks.
func EnsureRacks(c context.Context, cfgs []*config.DatacenterConfig, datacenterIds map[string]int64) error {
	r := &RacksTable{}
	r.datacenters = datacenterIds
	if err := r.fetch(c); err != nil {
		return errors.Annotate(err, "failed to fetch racks").Err()
	}
	if err := r.computeChanges(c, cfgs); err != nil {
		return errors.Annotate(err, "failed to compute changes").Err()
	}
	if err := r.add(c); err != nil {
		return errors.Annotate(err, "failed to add racks").Err()
	}
	if err := r.remove(c); err != nil {
		return errors.Annotate(err, "failed to remove racks").Err()
	}
	if err := r.update(c); err != nil {
		return errors.Annotate(err, "failed to update racks").Err()
	}
	return nil
}
