// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package backend

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"

	schema "go.chromium.org/luci/gce/api/bigquery/v1"
	"go.chromium.org/luci/gce/appengine/backend/internal/metrics"
	"go.chromium.org/luci/gce/appengine/model"
)

const (
	// projectID is the hosting project of the BigQuery tables. Default is the
	// current project.
	projectID = ""

	// dataset is the BigQuery dataset name.
	dataset = "datastore_snapshot" // '-' isn't a valid character.
)

// newBQDataset gets the interface to operate the BigQuery dataset.
func newBQDataset(c context.Context) (bqDataset, error) {
	prj := projectID
	if prj == "" {
		prj = info.AppID(c)
	}
	client, err := bigquery.NewClient(c, prj)
	if err != nil {
		return nil, errors.Fmt("new BQ dataset: %w", err)
	}
	return &realBQDataset{client: client, dsID: dataset}, nil
}

// uploadToBQ uploads rows of data from datastore to BQ.
func uploadToBQ(c context.Context, ds bqDataset) error {
	var entries = []struct { // all data to dump
		datastoreKind string
		bqTableName   string
		dumpFunc      func(c context.Context) ([]proto.Message, error)
	}{
		{
			datastoreKind: "InstanceCount",
			bqTableName:   "instance_count",
			dumpFunc:      dumpInstanceCount,
		},
		{
			datastoreKind: model.ConfigKind,
			bqTableName:   "config",
			dumpFunc:      dumpConfig,
		},
	}

	errs := []error{}
	for _, d := range entries {
		rows, err := d.dumpFunc(c)
		if err != nil {
			errs = append(errs, errors.Fmt("upload to BQ: %w", err))
			continue
		}
		if l := len(rows); l == 0 {
			logging.Debugf(c, "skip %q as no rows", d.datastoreKind)
			continue
		}
		if err := ds.putToTable(c, d.bqTableName, rows); err != nil {
			return errors.Fmt("upload to BQ (%q): %w", d.datastoreKind, err)
		} else {
			logging.Debugf(c, "dumped %d rows of %q", len(rows), d.datastoreKind)
		}
	}
	return errors.Join(errs...)
}

// bqDataset is the interface to operate BigQuery dataset.
// We have this interface mainly for better unit tests.
type bqDataset interface {
	putToTable(context.Context, string, []proto.Message) error
}

type realBQDataset struct {
	client *bigquery.Client
	dsID   string
}

// putToTable puts the src data to the named BigQuery table.
func (r *realBQDataset) putToTable(c context.Context, table string, src []proto.Message) error {
	up := bq.NewUploader(c, r.client, r.dsID, table)
	up.SkipInvalidRows = true
	up.IgnoreUnknownValues = true
	return up.Put(c, src...)
}

// dumpInstanceCount dumps InstanceCount datastore to a slice.
func dumpInstanceCount(c context.Context) ([]proto.Message, error) {
	rows := []proto.Message{}
	now := timestamppb.New(time.Now())
	q := datastore.NewQuery("InstanceCount")
	if err := datastore.Run(c, q, func(ic *metrics.InstanceCount) {
		configured := []*schema.ConfiguredCount{}
		for _, c := range ic.Configured {
			configured = append(configured, &schema.ConfiguredCount{Project: c.Project, Count: int64(c.Count)})
		}

		created := []*schema.CreatedCount{}
		for _, c := range ic.Created {
			created = append(created, &schema.CreatedCount{Project: c.Project, Zone: c.Zone, Count: int64(c.Count)})
		}

		connected := []*schema.ConnectedCount{}
		for _, c := range ic.Connected {
			connected = append(connected, &schema.ConnectedCount{Project: c.Project, Zone: c.Zone, Count: int64(c.Count), Server: c.Server})
		}

		r := &schema.InstanceCountRow{
			SnapshotTime:    now,
			Prefix:          ic.Prefix,
			Computed:        timestamppb.New(ic.Computed),
			ConfiguredCount: configured,
			CreatedCount:    created,
			ConnectedCount:  connected,
		}
		rows = append(rows, r)
	}); err != nil {
		return nil, errors.Fmt("dump InstanceCount: %w", err)
	}
	return rows, nil
}

// dumpConfig dumps Config datastore to a slice.
func dumpConfig(c context.Context) ([]proto.Message, error) {
	now := timestamppb.New(time.Now())
	rows := make([]proto.Message, 0)
	kind := model.ConfigKind
	q := datastore.NewQuery(kind)
	if err := datastore.Run(c, q, func(cfg *model.Config) {
		r := &schema.ConfigRow{
			SnapshotTime: now,
			Config:       cfg.Config,
		}
		rows = append(rows, r)
	}); err != nil {
		return nil, errors.Fmt("dump Config: %w", err)
	}
	return rows, nil
}
