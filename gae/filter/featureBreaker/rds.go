// Copyright 2015 The LUCI Authors.
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

package featureBreaker

import (
	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"
)

// DatastoreFeatures is a list of datastore features that can be "broken".
var DatastoreFeatures = []string{
	"AllocateIDs",
	"DecodeCursor",
	"Run",
	"Count",
	"BeginTransaction",
	"CommitTransaction",
	"DeleteMulti",
	"GetMulti",
	"PutMulti",
}

type dsState struct {
	*state

	c   context.Context
	rds ds.RawInterface
}

func (r *dsState) AllocateIDs(keys []*ds.Key, cb ds.NewKeyCB) error {
	if len(keys) == 0 {
		return nil
	}
	return r.run(r.c, func() error {
		return r.rds.AllocateIDs(keys, cb)
	})
}

func (r *dsState) DecodeCursor(s string) (ds.Cursor, error) {
	curs := ds.Cursor(nil)
	err := r.run(r.c, func() (err error) {
		curs, err = r.rds.DecodeCursor(s)
		return
	})
	return curs, err
}

func (r *dsState) Run(q *ds.FinalizedQuery, cb ds.RawRunCB) error {
	return r.run(r.c, func() error {
		return r.rds.Run(q, cb)
	})
}

func (r *dsState) Count(q *ds.FinalizedQuery) (int64, error) {
	count := int64(0)
	err := r.run(r.c, func() (err error) {
		count, err = r.rds.Count(q)
		return
	})
	return count, err
}

func (r *dsState) RunInTransaction(f func(c context.Context) error, opts *ds.TransactionOptions) error {
	// Note: we intentionally don't break RunInTransaction itself, but break
	// BeginTransaction/CommitTransaction separately instead.
	return r.rds.RunInTransaction(func(txnc context.Context) error {
		if err := r.BeginTransaction(txnc); err != nil {
			return err
		}
		if err := f(txnc); err != nil {
			return err
		}
		return r.CommitTransaction(txnc)
	}, opts)
}

// BeginTransaction is exposed as a "breakable" hook to simulate bad transaction
// initiation.
//
// It not a part of RawDatastore interface, but can nevertheless be overridden
// with BreakFeature to simulate "BeginTransaction" RPC errors.
//
// Unlike RunInTransaction (which is called once), BeginTransaction is called at
// the beginning of each individual transaction retry.
func (r *dsState) BeginTransaction(c context.Context) error {
	return r.run(c, func() error { return nil })
}

// CommitTransaction is exposed as a "breakable" hook to simulate bad
// transaction commits.
//
// It not a part of RawDatastore interface, but can nevertheless be overridden
// with BreakFeature to simulate transaction commit errors.
func (r *dsState) CommitTransaction(c context.Context) error {
	return r.run(c, func() error { return nil })
}

// TODO(iannucci): Allow the user to specify a multierror which will propagate
// to the callback correctly.

func (r *dsState) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	if len(keys) == 0 {
		return nil
	}
	return r.run(r.c, func() error {
		return r.rds.DeleteMulti(keys, cb)
	})
}

func (r *dsState) GetMulti(keys []*ds.Key, meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	if len(keys) == 0 {
		return nil
	}
	return r.run(r.c, func() error {
		return r.rds.GetMulti(keys, meta, cb)
	})
}

func (r *dsState) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) error {
	if len(keys) == 0 {
		return nil
	}
	return r.run(r.c, func() (err error) {
		return r.rds.PutMulti(keys, vals, cb)
	})
}

func (r *dsState) WithoutTransaction() context.Context {
	return r.rds.WithoutTransaction()
}

func (r *dsState) CurrentTransaction() ds.Transaction {
	return r.rds.CurrentTransaction()
}

func (r *dsState) Constraints() ds.Constraints { return r.rds.Constraints() }

func (r *dsState) GetTestable() ds.Testable {
	return r.rds.GetTestable()
}

// FilterRDS installs a featureBreaker datastore filter in the context.
func FilterRDS(c context.Context, defaultError error) (context.Context, FeatureBreaker) {
	state := newState(defaultError)
	return ds.AddRawFilters(c, func(ic context.Context, rawDatastore ds.RawInterface) ds.RawInterface {
		return &dsState{state, ic, rawDatastore}
	}), state
}
