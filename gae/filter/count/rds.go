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

package count

import (
	"golang.org/x/net/context"

	ds "go.chromium.org/gae/service/datastore"
)

// DSCounter is the counter object for the datastore service.
type DSCounter struct {
	AllocateIDs      Entry
	DecodeCursor     Entry
	RunInTransaction Entry
	Run              Entry
	Count            Entry
	DeleteMulti      Entry
	GetMulti         Entry
	PutMulti         Entry
}

type dsCounter struct {
	c *DSCounter

	ds ds.RawInterface
}

var _ ds.RawInterface = (*dsCounter)(nil)

func (r *dsCounter) AllocateIDs(keys []*ds.Key, cb ds.NewKeyCB) error {
	return r.c.AllocateIDs.up(r.ds.AllocateIDs(keys, cb))
}

func (r *dsCounter) DecodeCursor(s string) (ds.Cursor, error) {
	cursor, err := r.ds.DecodeCursor(s)
	return cursor, r.c.DecodeCursor.up(err)
}

func (r *dsCounter) Run(q *ds.FinalizedQuery, cb ds.RawRunCB) error {
	return r.c.Run.upFilterStop(r.ds.Run(q, cb))
}

func (r *dsCounter) Count(q *ds.FinalizedQuery) (int64, error) {
	count, err := r.ds.Count(q)
	return count, r.c.Count.up(err)
}

func (r *dsCounter) RunInTransaction(f func(context.Context) error, opts *ds.TransactionOptions) error {
	return r.c.RunInTransaction.up(r.ds.RunInTransaction(f, opts))
}

func (r *dsCounter) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	return r.c.DeleteMulti.upFilterStop(r.ds.DeleteMulti(keys, cb))
}

func (r *dsCounter) GetMulti(keys []*ds.Key, meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	return r.c.GetMulti.upFilterStop(r.ds.GetMulti(keys, meta, cb))
}

func (r *dsCounter) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) error {
	return r.c.PutMulti.upFilterStop(r.ds.PutMulti(keys, vals, cb))
}

func (r *dsCounter) CurrentTransaction() ds.Transaction {
	return r.ds.CurrentTransaction()
}
func (r *dsCounter) WithoutTransaction() context.Context {
	return r.ds.WithoutTransaction()
}

func (r *dsCounter) Constraints() ds.Constraints {
	return r.ds.Constraints()
}

func (r *dsCounter) GetTestable() ds.Testable {
	return r.ds.GetTestable()
}

// FilterRDS installs a counter datastore filter in the context.
func FilterRDS(c context.Context) (context.Context, *DSCounter) {
	state := &DSCounter{}
	return ds.AddRawFilters(c, func(ic context.Context, ds ds.RawInterface) ds.RawInterface {
		return &dsCounter{state, ds}
	}), state
}

// upFilterStop wraps up, handling the special case datastore.Stop error.
// datastore.Stop will pass through this function, but, unlike other error
// codes, will be counted as a success.
func (e *Entry) upFilterStop(err error) error {
	upErr := err
	if upErr == ds.Stop {
		upErr = nil
	}
	e.up(upErr)
	return err
}
