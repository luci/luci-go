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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// Switch represents a switch.
type Switch struct {
	config.SwitchConfig
	RackId int64
	Id     int64
}

// SwitchesTable represents the table of switches in the database.
type SwitchesTable struct {
	// racks is a map of rack name to ID in the database.
	racks map[string]int64
	// current is the slice of switches in the database.
	current []*Switch

	// additions is a slice of switches pending addition to the database.
	additions []*Switch
	// removals is a slice of switches pending removal from the database.
	removals []*Switch
	// updates is a slice of switches pending update in the database.
	updates []*Switch
}

// fetch fetches the switches from the database.
func (t *SwitchesTable) fetch(c context.Context) error {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT id, name, description, ports, rack_id
		FROM switches
	`)
	if err != nil {
		return errors.Annotate(err, "failed to select switches").Err()
	}
	defer rows.Close()
	for rows.Next() {
		s := &Switch{}
		if err := rows.Scan(&s.Id, &s.Name, &s.Description, &s.Ports, &s.RackId); err != nil {
			return errors.Annotate(err, "failed to scan switch").Err()
		}
		t.current = append(t.current, s)
	}
	return nil
}

// needsUpdate returns true if the given row needs to be updated to match the given config.
func (*SwitchesTable) needsUpdate(row, cfg *Switch) bool {
	return row.Ports != cfg.Ports || row.Description != cfg.Description || row.RackId != cfg.RackId
}

// computeChanges computes the changes that need to be made to the switches in the database.
func (t *SwitchesTable) computeChanges(c context.Context, datacenters []*config.DatacenterConfig) error {
	cfgs := make(map[string]*Switch, len(datacenters))
	for _, dc := range datacenters {
		for _, rack := range dc.Rack {
			for _, cfg := range rack.Switch {
				id, ok := t.racks[rack.Name]
				if !ok {
					return errors.Reason("failed to determine rack ID for switch %q: unknown rack %q", cfg.Name, rack.Name).Err()
				}
				cfgs[cfg.Name] = &Switch{
					SwitchConfig: config.SwitchConfig{
						Name:        cfg.Name,
						Description: cfg.Description,
						Ports:       cfg.Ports,
					},
					RackId: id,
				}
			}
		}
	}

	for _, s := range t.current {
		if cfg, ok := cfgs[s.Name]; ok {
			// Switch found in the config.
			if t.needsUpdate(s, cfg) {
				// Switch doesn't match the config.
				cfg.Id = s.Id
				t.updates = append(t.updates, cfg)
			}
			// Record that the switch config has been seen.
			delete(cfgs, cfg.Name)
		} else {
			// Switch not found in the config.
			t.removals = append(t.removals, s)
		}
	}

	// Switches remaining in the map are present in the config but not the database.
	// Iterate deterministically over the slices to determine which switches need to be added.
	for _, dc := range datacenters {
		for _, rack := range dc.Rack {
			for _, cfg := range rack.Switch {
				if s, ok := cfgs[cfg.Name]; ok {
					t.additions = append(t.additions, s)
				}
			}
		}
	}
	return nil
}

// add adds all switches pending addition to the database, clearing pending additions.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *SwitchesTable) add(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.additions) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		INSERT INTO switches (name, description, ports, rack_id)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Add each switch to the database, and update the slice of switches with each addition.
	for len(t.additions) > 0 {
		s := t.additions[0]
		result, err := stmt.ExecContext(c, s.Name, s.Description, s.Ports, s.RackId)
		if err != nil {
			return errors.Annotate(err, "failed to add switch %q", s.Name).Err()
		}
		t.current = append(t.current, s)
		t.additions = t.additions[1:]
		logging.Infof(c, "Added switch %q", s.Name)
		s.Id, err = result.LastInsertId()
		if err != nil {
			return errors.Annotate(err, "failed to get switch ID %q", s.Name).Err()
		}
	}
	return nil
}

// remove removes all switches pending removal from the database, clearing pending removals.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *SwitchesTable) remove(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.removals) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		DELETE FROM switches
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Remove each switch from the database. It's more efficient to update the slice of
	// switches once at the end rather than for each removal, so use a defer.
	removed := make(map[int64]struct{}, len(t.removals))
	defer func() {
		var switches []*Switch
		for _, s := range t.current {
			if _, ok := removed[s.Id]; !ok {
				switches = append(switches, s)
			}
		}
		t.current = switches
	}()
	for len(t.removals) > 0 {
		s := t.removals[0]
		if _, err := stmt.ExecContext(c, s.Id); err != nil {
			// Defer ensures the slice of switches is updated even if we exit early.
			return errors.Annotate(err, "failed to remove switch %q", s.Name).Err()
		}
		removed[s.Id] = struct{}{}
		t.removals = t.removals[1:]
		logging.Infof(c, "Removed switch %q", s.Name)
	}
	return nil
}

// update updates all switches pending update in the database, clearing pending updates.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *SwitchesTable) update(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.updates) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		UPDATE switches
		SET description = ?, ports = ?, rack_id = ?
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Update each switch in the database. It's more efficient to update the slice of
	// switches once at the end rather than for each update, so use a defer.
	updated := make(map[int64]*Switch, len(t.updates))
	defer func() {
		for _, s := range t.current {
			if u, ok := updated[s.Id]; ok {
				s.Description = u.Description
				s.Ports = u.Ports
				s.RackId = u.RackId
			}
		}
	}()
	for len(t.updates) > 0 {
		s := t.updates[0]
		if _, err := stmt.ExecContext(c, s.Description, s.Ports, s.RackId, s.Id); err != nil {
			return errors.Annotate(err, "failed to update switch %q", s.Name).Err()
		}
		updated[s.Id] = s
		t.updates = t.updates[1:]
		logging.Infof(c, "Updated switch %q", s.Name)
	}
	return nil
}

// ids returns a map of switch names to IDs.
func (t *SwitchesTable) ids(c context.Context) map[string]int64 {
	switches := make(map[string]int64, len(t.current))
	for _, s := range t.current {
		switches[s.Name] = s.Id
	}
	return switches
}

// EnsureSwitches ensures the database contains exactly the given switches.
func EnsureSwitches(c context.Context, cfgs []*config.DatacenterConfig, rackIds map[string]int64) error {
	t := &SwitchesTable{}
	t.racks = rackIds
	if err := t.fetch(c); err != nil {
		return errors.Annotate(err, "failed to fetch switches").Err()
	}
	if err := t.computeChanges(c, cfgs); err != nil {
		return errors.Annotate(err, "failed to compute changes").Err()
	}
	if err := t.add(c); err != nil {
		return errors.Annotate(err, "failed to add switches").Err()
	}
	if err := t.remove(c); err != nil {
		return errors.Annotate(err, "failed to remove switches").Err()
	}
	if err := t.update(c); err != nil {
		return errors.Annotate(err, "failed to update switches").Err()
	}
	return nil
}
