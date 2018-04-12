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
	"database/sql"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
)

// RackKVM represents a kvm_id column in a row in the racks table.
type RackKVM struct {
	config.Rack
	Id    int64
	KVMId sql.NullInt64
}

// RackKVMsTable represents the table of kvm_ids for racks in the database.
type RackKVMsTable struct {
	// kvms is a map of KVM name to ID in the database.
	kvms map[string]int64
	// current is the slice of rack kvm_ids in the database.
	current []*RackKVM

	// updates is a slice of rack kvm_ids pending update in the database.
	updates []*RackKVM
}

// fetch fetches the rack kvm_ids from the database.
func (t *RackKVMsTable) fetch(c context.Context) error {
	db := database.Get(c)
	rows, err := db.QueryContext(c, `
		SELECT id, name, kvm_id
		FROM racks
	`)
	if err != nil {
		return errors.Annotate(err, "failed to select racks").Err()
	}
	defer rows.Close()
	for rows.Next() {
		rack := &RackKVM{}
		if err := rows.Scan(&rack.Id, &rack.Name, &rack.KVMId); err != nil {
			return errors.Annotate(err, "failed to scan racks").Err()
		}
		t.current = append(t.current, rack)
	}
	return nil
}

// needsUpdate returns true if the given row needs to be updated to match the given config.
func (*RackKVMsTable) needsUpdate(row, cfg *RackKVM) bool {
	return row.KVMId != cfg.KVMId
}

// computeChanges computes the changes that need to be made to the rack kvm_ids in the database.
func (t *RackKVMsTable) computeChanges(c context.Context, datacenters []*config.Datacenter) error {
	cfgs := make(map[string]*RackKVM, len(datacenters))
	for _, dc := range datacenters {
		for _, cfg := range dc.Rack {
			cfgs[cfg.Name] = &RackKVM{
				Rack: config.Rack{
					Name: cfg.Name,
				},
			}
			if cfg.Kvm != "" {
				id, ok := t.kvms[cfg.Kvm]
				if !ok {
					return errors.Reason("failed to determine KVM ID for rack %q: unknown KVM %q", cfg.Name, cfg.Kvm).Err()
				}
				cfgs[cfg.Name].KVMId.Int64 = id
				cfgs[cfg.Name].KVMId.Valid = true
			}
		}
	}

	for _, rack := range t.current {
		if cfg, ok := cfgs[rack.Name]; ok {
			// Rack found in the config.
			if t.needsUpdate(rack, cfg) {
				// Rack doesn't match the config.
				cfg.Id = rack.Id
				t.updates = append(t.updates, cfg)
			}
			// Record that the rack config has been seen.
			delete(cfgs, cfg.Name)
		}
	}
	return nil
}

// update updates all rack kvm_ids pending update in the database, clearing pending updates.
// No-op unless computeChanges was called first. Idempotent until computeChanges is called again.
func (t *RackKVMsTable) update(c context.Context) error {
	// Avoid using the database connection to prepare unnecessary statements.
	if len(t.updates) == 0 {
		return nil
	}

	db := database.Get(c)
	stmt, err := db.PrepareContext(c, `
		UPDATE racks
		SET kvm_id = ?
		WHERE id = ?
	`)
	if err != nil {
		return errors.Annotate(err, "failed to prepare statement").Err()
	}
	defer stmt.Close()

	// Update each rack in the table. It's more efficient to update the slice of
	// racks once at the end rather than for each update, so use a defer.
	updated := make(map[int64]*RackKVM, len(t.updates))
	defer func() {
		for _, rack := range t.current {
			if u, ok := updated[rack.Id]; ok {
				rack.KVMId = u.KVMId
			}
		}
	}()
	for len(t.updates) > 0 {
		rack := t.updates[0]
		if _, err := stmt.ExecContext(c, rack.KVMId, rack.Id); err != nil {
			return errors.Annotate(err, "failed to update rack %q", rack.Name).Err()
		}
		updated[rack.Id] = rack
		t.updates = t.updates[1:]
		logging.Infof(c, "Updated KVM for rack %q", rack.Name)
	}
	return nil
}

// EnsureRackKVMs ensures the database contains exactly the given rack kvm_ids.
func EnsureRackKVMs(c context.Context, cfgs []*config.Datacenter, kvmIds map[string]int64) error {
	t := &RackKVMsTable{}
	t.kvms = kvmIds
	if err := t.fetch(c); err != nil {
		return errors.Annotate(err, "failed to fetch racks").Err()
	}
	if err := t.computeChanges(c, cfgs); err != nil {
		return errors.Annotate(err, "failed to compute changes").Err()
	}
	if err := t.update(c); err != nil {
		return errors.Annotate(err, "failed to update racks").Err()
	}
	return nil
}
