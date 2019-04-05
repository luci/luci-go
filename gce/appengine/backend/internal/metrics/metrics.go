// Copyright 2019 The LUCI Authors.
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

// Package metrics includes tsmon metric support for internal use by backend.
package metrics

import (
	"context"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"

	"go.chromium.org/luci/gce/appengine/model"
)

var (
	creationFailures = metric.NewCounter(
		"gce/failures/creation",
		"The number of failures during GCE instance creation.",
		nil,
		field.String("prefix"),
		field.String("project"),
		field.String("zone"),
	)
)

// UpdateFailures increments failure counters.
func UpdateFailures(c context.Context, creations int, vm *model.VM) {
	creationFailures.Add(c, int64(creations), vm.Prefix, vm.Attributes.GetProject(), vm.Attributes.GetZone())
}

var (
	configuredInstances = metric.NewInt(
		"gce/instances/configured",
		"The number of GCE instances configured to exist.",
		nil,
		field.String("prefix"),
		field.String("project"),
	)

	connectedInstances = metric.NewInt(
		"gce/instances/connected",
		"The number of GCE instances connected to Swarming.",
		nil,
		field.String("prefix"),
		field.String("project"),
		field.String("server"),
		field.String("zone"),
	)

	createdInstances = metric.NewInt(
		"gce/instances/created",
		"The number of GCE instances created.",
		nil,
		field.String("prefix"),
		field.String("project"),
		field.String("zone"),
	)
)

// instanceCount encapsulates a count of connected and created VMs.
type instanceCount struct {
	Connected int
	Created   int
	Project   string
	Server    string
	Zone      string
}

// InstanceCounter tracks counts of connected and created VMs for a common
// prefix. Create a new one with make(InstanceCounter).
type InstanceCounter map[string]map[string]map[string]*instanceCount

// get returns the *instanceCount associated with the given project, server, and
// zone, creating it if it doesn't exist.
func (ic InstanceCounter) get(project, server, zone string) *instanceCount {
	if _, ok := ic[project]; !ok {
		ic[project] = make(map[string]map[string]*instanceCount)
	}
	if _, ok := ic[project][server]; !ok {
		ic[project][server] = make(map[string]*instanceCount)
	}
	if _, ok := ic[project][server][zone]; !ok {
		ic[project][server][zone] = &instanceCount{
			Project: project,
			Server:  server,
			Zone:    zone,
		}
	}
	return ic[project][server][zone]
}

// Connected increments by one the count of connected VMs for the given project,
// server, and zone.
func (ic InstanceCounter) Connected(project, server, zone string) {
	ic.get(project, server, zone).Connected++
}

// Created increments by one the count of created VMs for the given project,
// server, and zone.
func (ic InstanceCounter) Created(project, server, zone string) {
	ic.get(project, server, zone).Created++
}

// Update updates metrics for all known counts of VMs for the given prefix.
func (ic InstanceCounter) Update(c context.Context, prefix string) error {
	n := &InstanceCount{
		// Prefixes are globally unique, so we can use them as IDs.
		ID:       prefix,
		Computed: clock.Now(c).UTC(),
		Counts:   make([]instanceCount, 0),
		Prefix:   prefix,
	}
	for _, srvs := range ic {
		for _, zones := range srvs {
			for _, count := range zones {
				n.Counts = append(n.Counts, *count)
			}
		}
	}
	if err := datastore.Put(c, n); err != nil {
		return errors.Annotate(err, "failed to store count").Err()
	}
	return nil
}

// InstanceCount is a root entity representing a count for instances with a
// common prefix.
type InstanceCount struct {
	// _extra is where unknown properties are put into memory.
	// Extra properties are not written to the datastore.
	_extra datastore.PropertyMap `gae:"-,extra"`
	// _kind is the entity's kind in the datastore.
	_kind string `gae:"$kind,InstanceCount"`
	// ID is the unique identifier for this count.
	ID string `gae:"$id"`
	// Prefix is the prefix for this count.
	Prefix string `gae:"prefix"`
	// Computed is the time this count was computed.
	Computed time.Time `gae:"computed"`
	// Counts is a slice of instanceCounts.
	Counts []instanceCount `gae:"counts,noindex"`
}

// UpdateConfiguredInstances sets configured GCE instance metrics.
func UpdateConfiguredInstances(c context.Context, configured int, prefix, project string) {
	configuredInstances.Set(c, int64(configured), prefix, project)
}

// updateInstances sets GCE instance metrics.
func updateInstances(c context.Context) {
	now := clock.Now(c)
	q := datastore.NewQuery("InstanceCount").Order("computed")
	if err := datastore.Run(c, q, func(n *InstanceCount) {
		if now.Sub(n.Computed) > 10*time.Minute {
			logging.Debugf(c, "deleting outdated count %q", n.Prefix)
			if err := datastore.Delete(c, n); err != nil {
				logging.Errorf(c, "%s", err)
			}
			return
		}
		for _, count := range n.Counts {
			connectedInstances.Set(c, int64(count.Connected), n.Prefix, count.Project, count.Server, count.Zone)
			createdInstances.Set(c, int64(count.Created), n.Prefix, count.Project, count.Zone)
		}
	}); err != nil {
		errors.Log(c, errors.Annotate(err, "failed to fetch counts").Err())
	}
}

func init() {
	tsmon.RegisterGlobalCallback(updateInstances, connectedInstances, createdInstances)
}

var (
	quotaLimit = metric.NewFloat(
		"gce/quota/limit",
		"The GCE quota limit for a particular metric.",
		nil,
		field.String("metric"),
		field.String("project"),
		field.String("region"),
	)

	quotaRemaining = metric.NewFloat(
		"gce/quota/remaining",
		"The remaining GCE quota for a particular metric.",
		nil,
		field.String("metric"),
		field.String("project"),
		field.String("region"),
	)

	quotaUsage = metric.NewFloat(
		"gce/quota/usage",
		"The GCE quota being used for a particular metric.",
		nil,
		field.String("metric"),
		field.String("project"),
		field.String("region"),
	)
)

// UpdateQuota sets GCE quota metrics.
func UpdateQuota(c context.Context, limit, usage float64, metric, project, region string) {
	quotaLimit.Set(c, limit, metric, project, region)
	quotaRemaining.Set(c, limit-usage, metric, project, region)
	quotaUsage.Set(c, usage, metric, project, region)
}
