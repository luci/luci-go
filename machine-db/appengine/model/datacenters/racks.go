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

package datacenters

import (
	"fmt"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/machine-db/api/config/v1"
	"go.chromium.org/luci/machine-db/api/crimson/v1"
	"go.chromium.org/luci/machine-db/appengine/database"
	"go.chromium.org/luci/machine-db/appengine/model"
	"go.chromium.org/luci/server/auth"

	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// Rack represents a rack.
type Rack struct {
	*config.RackConfig
	Datacenter string
	Id         int64
}

// Racks represents the table of racks in the database.
type Racks struct {
	// additions is a list of racks pending addition to the database.
	additions []*Rack
	// datacenters is a map of datacenter name to ID in the database.
	datacenters map[string]int64
	// racks is the list of racks in the database.
	racks []*Rack
	// removals is a list of racks pending removal from the database.
	removals []string
	// updates is a list of racks pending update in the database.
	updates []*Rack
}

// fetch fetches the racks from the database.
func (r *Racks) fetch(c context.Context) error {
	db := database.Get(c)
	rows, err := db.QueryContext(c, "SELECT racks.id, racks.name, racks.description, datacenters.name FROM racks, datacenters WHERE racks.datacenter_id = datacenters.id")
	if err != nil {
		return errors.Annotate(err, "failed to select racks").Err()
	}
	defer rows.Close()
	for rows.Next() {
		rack := &Rack{
			RackConfig: &config.RackConfig{},
			Datacenter: "",
			Id:         -1,
		}
		if err := rows.Scan(&rack.Id, &rack.Name, &rack.Description, &rack.Datacenter); err != nil {
			return errors.Annotate(err, "failed to scan rack").Err()
		}
		r.racks = append(r.racks, rack)
	}
	return nil
}

// computeChanges computes the changes that need to be made to the racks in the database.
func (r *Racks) computeChanges(c context.Context, datacenters []*config.DatacenterConfig) error {
	cfgs := make(map[string]*Rack, len(datacenters))
	for _, dc := range datacenters {
		for _, cfg := range dc.Rack {
			cfgs[cfg.Name] = &Rack{
				RackConfig: cfg,
				Id:         -1,
				Datacenter: dc.Name,
			}
		}
	}

	for _, rack := range r.racks {
		if cfg, ok := cfgs[rack.Name]; ok {
			// Rack found in the config.
			if rack.Description != cfg.Description || rack.Datacenter != cfg.Datacenter {
				// Rack doesn't match the config.
				r.updates = append(r.updates, &Rack{
					RackConfig: cfg.RackConfig,
					Datacenter: rack.Datacenter,
					Id:         rack.Id,
				})
			}
			// Record that the rack config has been seen.
			delete(cfgs, cfg.Name)
		} else {
			// Rack not found in the config.
			r.removals = append(r.removals, rack.Name)
		}
	}

	// Racks remaining in the map are present in the config but not the database.
	// Iterate deterministically over the array to determine which racks need to be added.
	for _, dc := range datacenters {
		for _, cfg := range dc.Rack {
			if _, ok := cfgs[cfg.Name]; ok {
				r.additions = append(r.additions, &Rack{
					RackConfig: cfg,
					Datacenter: dc.Name,
					Id:         -1,
				})
			}
		}
	}
	return nil
}

// add adds all racks pending addition to the database.
func (r *Racks) add(c context.Context) error {
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

	// Add each rack to the database, and update the list of racks with each addition.
	for len(r.additions) > 0 {
		rack := r.additions[0]
		id, ok := r.datacenters[rack.Datacenter]
		if !ok {
			return errors.Annotate(fmt.Errorf("unknown datacenter: %s", rack.Datacenter), "failed to determine datacenter ID for rack: %s", rack.Name).Err()
		}
		result, err := stmt.ExecContext(c, rack.Name, rack.Description, id)
		if err != nil {
			return errors.Annotate(err, "failed to add rack: %s", rack.Name).Err()
		}
		r.racks = append(r.racks, rack)
		r.additions = r.additions[1:]
		logging.Infof(c, "Added rack: %s", rack.Name)
		rack.Id, err = result.LastInsertId()
		if err != nil {
			return errors.Annotate(err, "failed to get rack ID: %s", rack.Name).Err()
		}
	}
	return nil
}

// remove removes all racks pending removal from the database.
func (r *Racks) remove(c context.Context) error {
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

	// Remove each rack from the database. It's more efficient to update the list of
	// racks once at the end rather than for each removal, so use a defer.
	removed := stringset.New(len(r.removals))
	defer func() {
		var racks []*Rack
		for _, rack := range r.racks {
			if !removed.Has(rack.Name) {
				racks = append(racks, rack)
			}
		}
		r.racks = racks
	}()
	for len(r.removals) > 0 {
		rack := r.removals[0]
		if _, err := stmt.ExecContext(c, rack); err != nil {
			// Defer ensures the list of racks is updated even if we exit early.
			return errors.Annotate(err, "failed to remove rack: %s", rack).Err()
		}
		removed.Add(rack)
		r.removals = r.removals[1:]
		logging.Infof(c, "Removed rack: %s", rack)
	}
	return nil
}

// update updates all racks pending update in the database.
func (r *Racks) update(c context.Context) error {
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

	// Update each rack in the database. It's more efficient to update the list of
	// racks once at the end rather than for each update, so use a defer.
	updated := make(map[string]*Rack, len(r.updates))
	defer func() {
		for _, rack := range r.racks {
			if _, ok := updated[rack.Name]; ok {
				rack.Description = updated[rack.Name].Description
				rack.Datacenter = updated[rack.Name].Datacenter
			}
		}
	}()
	for len(r.updates) > 0 {
		rack := r.updates[0]
		id, ok := r.datacenters[rack.Datacenter]
		if !ok {
			return errors.Annotate(fmt.Errorf("unknown datacenter: %s", rack.Datacenter), "failed to determine datacenter ID for rack: %s", rack.Name).Err()
		}
		if _, err := stmt.ExecContext(c, rack.Description, id, rack.Id); err != nil {
			return errors.Annotate(err, "failed to update rack: %s", rack.Name).Err()
		}
		updated[rack.Name] = rack
		r.updates = r.updates[1:]
		logging.Infof(c, "Updated rack: %s", rack.Name)
	}
	return nil
}

// ids returns a map of rack names to IDs.
func (r *Racks) ids(c context.Context) map[string]int64 {
	racks := make(map[string]int64, len(r.racks))
	for _, rack := range r.racks {
		racks[rack.Name] = rack.Id
	}
	return racks
}

// EnsureRacks ensures the database contains exactly the given racks.
func EnsureRacks(c context.Context, cfgs []*config.DatacenterConfig, datacenterIds map[string]int64) error {
	r := &Racks{}
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

// RacksServer handles datacenter RPCs.
type RacksServer struct {
}

// IsAuthorized returns whether the current user is authorized to use the RacksServer API.
func (r *RacksServer) IsAuthorized(c context.Context) (bool, error) {
	// TODO(smut): Create other groups for this.
	is, err := auth.IsMember(c, "administrators")
	if err != nil {
		return false, errors.Annotate(err, "failed to check group membership").Err()
	}
	return is, err
}

// GetRacks handles a request to retrieve racks.
func (r *RacksServer) GetRacks(c context.Context, req *crimson.RacksRequest) (*crimson.RacksResponse, error) {
	switch authorized, err := r.IsAuthorized(c); {
	case err != nil:
		return nil, model.InternalRPCError(c, err)
	case !authorized:
		return nil, grpc.Errorf(codes.PermissionDenied, "Unauthorized user")
	}
	names := stringset.New(len(req.Names))
	for _, name := range req.Names {
		names.Add(name)
	}
	datacenters := stringset.New(len(req.Datacenters))
	for _, dc := range req.Datacenters {
		datacenters.Add(dc)
	}
	racks, err := r.getRacks(c, names, datacenters)
	if err != nil {
		return nil, model.InternalRPCError(c, err)
	}
	return &crimson.RacksResponse{
		Racks: racks,
	}, nil
}

// getRacks returns a list of racks in the database.
func (r *RacksServer) getRacks(c context.Context, names stringset.Set, datacenters stringset.Set) ([]*crimson.Rack, error) {
	db := database.Get(c)
	rows, err := db.QueryContext(c, "SELECT racks.name, racks.description, datacenters.name FROM racks, datacenters WHERE racks.datacenter_id = datacenters.id")
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch racks").Err()
	}
	defer rows.Close()

	var racks []*crimson.Rack
	for rows.Next() {
		var name, description, datacenter string
		if err = rows.Scan(&name, &description, &datacenter); err != nil {
			return nil, errors.Annotate(err, "failed to fetch rack").Err()
		}
		if (names.Has(name) || names.Len() == 0) && (datacenters.Has(datacenter) || datacenters.Len() == 0) {
			racks = append(racks, &crimson.Rack{
				Name:        name,
				Description: description,
				Datacenter:  datacenter,
			})
		}
	}
	return racks, nil
}
