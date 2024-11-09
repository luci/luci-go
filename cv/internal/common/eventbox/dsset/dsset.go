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

// Package dsset implements a particular flavor of Datastore-on-Firestore backed
// set.
//
// Due to its internal structure, it requires some maintenance on behalf of the
// caller to periodically cleanup removed items (aka tombstones).
//
// Items added to the set should have unique IDs, at least for the duration of
// some configurable time interval, as defined by TombstonesDelay property.
// It means removed items can't be added back to the set right away (the set
// will think they are already there). This is required to make 'Add' operation
// idempotent.
//
// TombstonesDelay is assumed to be much larger than time scale of all "fast"
// processes in the system, in particular all List+Pop processes. For example,
// if List+Pop is expected to take 1 min, TombstonesDelay should be >> 1 min
// (e.g. 5 min). Setting TombstonesDelay to very large value is harmful though,
// since it may slow down 'List' and 'Pop' (by allowing more garbage that will
// have to be filtered out).
//
// Properties (where N is current size of the set):
//   - Batch 'Add', O(1) performance.
//   - Transactional consistent 'Pop' (1 QPS limit), O(N) performance.
//   - Non-transactional consistent 'List', O(N) performance.
//   - Popped items can't be re-added until their tombstones expire.
//
// These properties make dsset suitable for multiple producers, single consumer
// queues, where order of items is not important, each item has a unique
// identifier, and the queue size is small.
//
// Structurally dsset places 2 kinds of entities under provided Set's parent
// entity:
//   - items of the set.
//   - tombstones, recording deleted items.
//
// This code is a fork of dsset for classic Datastore, which had to work around
// 1 write per second per entity group limit using shards. See
// go.chromium.org/luci/scheduler/appengine/engine/dsset.
package dsset

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/tracing"
)

// Set holds a set of Items and uses tombstones to achieve idempotency of Add.
//
// Producers just call Add(...).
//
// The consumer must run more elaborate algorithm that ensures atomicity of
// 'Pop' and takes care of cleaning up of the garbage. This requires a mix of
// transactional and non-transactional actions:
//
//	listing, err := set.List(ctx)
//	if err != nil {
//	  return err
//	}
//
//	if err := dsset.CleanupGarbage(ctx, listing.Garbage); err != nil {
//	  return err
//	}
//
//	... Fetch any additional info associated with 'listing.Items' ...
//
//	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
//	  op, err := set.BeginPop(ctx, listing)
//	  if err != nil {
//	    return err
//	  }
//	  for _, itm := range listing.items {
//	    if op.Pop(item.ID) {
//	      // The item was indeed in the set and we've just removed it!
//	    } else {
//	      // Some other transaction has popped it already.
//	    }
//	  }
//	  return dsset.FinishPop(ctx, op)
//	}, nil)
//	return err
type Set struct {
	// Parent points to the datastore owning the set.
	//
	// Set's Datastore entities will be placed with this parent.
	Parent *datastore.Key
	// TombstonesDelay is how long to keep tombstones in the set.
	TombstonesDelay time.Duration
}

// Item is what's stored in the set.
type Item struct {
	ID    string // unique in time identifier of the item
	Value []byte // arbitrary value (<1 MB, but preferably much smaller)
}

// Garbage is a list of tombstones to cleanup.
type Garbage []*tombstone

// Listing is returned by 'List' call.
//
// It contains actual listing of items in the set, as well as a bunch of service
// information used by other operations ('CleanupGarbage' and 'Pop') to keep
// the set in a garbage-free and consistent state.
//
// The only way to construct a correct Listing is to call 'List' method.
//
// See comments for Set struct and List method for more info.
type Listing struct {
	Items   []Item  // all items in the set, in arbitrary order
	Garbage Garbage // tombstones that can be cleaned up now

	parent     *datastore.Key            // set's parent.
	producedAt time.Time                 // when 'List' call was initiated
	idToKey    map[string]*datastore.Key // ID -> datastore key to cleanup
}

// tombstone is a reference to a deleted item that still lingers in the set.
//
// Tombstones exist to make sure recently popped items do not reappear in the
// set if producers attempt to re-add them.
type tombstone struct {
	id        string         // deleted item ID
	storage   *datastore.Key // itemEntity to delete in 'CleanupGarbage'
	old       bool           // true if tombstone should be popped in 'Pop'
	cleanedUp bool           // true if 'CleanupGarbage' processed the tombstone
}

// Add idempotently adds a bunch of items to the set.
//
// If items with given keys are already in the set, or have been deleted
// recently, they won't be re-added. No error is returned in this case. When
// retrying the call like that, the caller is responsible to pass exact same
// Item.Value, otherwise 'List' may return random variant of the added item.
//
// If called outside of a transaction and the call fails, may add only some
// subset of items. Running inside a transaction makes this operation atomic.
//
// Returns only transient errors.
func (s *Set) Add(c context.Context, items []Item) error {
	// If added items have been popped already (they have tombstones), 'List' will
	// omit them as well.
	entities := make([]itemEntity, len(items))
	for i, itm := range items {
		entities[i] = itemEntity{
			ID:     itm.ID,
			Parent: s.Parent,
			Value:  itm.Value,
		}
	}
	return transient.Tag.Apply(datastore.Put(c, entities))
}

// List returns all items that are currently in the set (in arbitrary order),
// as well as a set of tombstones that points to items that were previously
// popped and can be cleaned up now.
//
// Returns error if it is called outside of transactions.
//
// The set of tombstones to cleanup should be passed to 'CleanupGarbage', and
// later to 'BeginPop' (via Listing), in that order. Not doing so will lead to
// accumulation of a garbage in the set that will slow down 'List' and 'Pop'.
//
// Returns only transient errors.
func (s *Set) List(ctx context.Context, maxEvents int) (l *Listing, err error) {
	switch {
	case datastore.CurrentTransaction(ctx) != nil:
		return nil, errors.New("dsset.Set.List must be called outside of a transaction")
	case maxEvents <= 0:
		return nil, fmt.Errorf("maxEvents must be >0, but %d given", maxEvents)
	}
	ctx, span := tracing.Start(ctx, "go.chromium.org/luci/cv/internal/eventbox/dsset/List")
	defer func() { tracing.End(span, err) }()

	now := clock.Now(ctx).UTC()

	// Fetch all items and all tombstones.
	tombsEntity := tombstonesEntity{Parent: s.Parent}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		err := datastore.Get(ctx, &tombsEntity)
		if err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
		return nil
	})

	var entities []*itemEntity
	eg.Go(func() error {
		q := datastore.NewQuery("dsset.Item").Ancestor(s.Parent).Limit(int32(maxEvents))
		return datastore.GetAll(ctx, q, &entities)
	})
	if err := eg.Wait(); err != nil {
		return nil, transient.Tag.Apply(err)
	}

	// Mapping "item ID" => "entity to delete to remove it". This is eventually
	// used by 'CleanupGarbage'.
	idToKey := map[string]*datastore.Key{}
	for _, e := range entities {
		idToKey[e.ID] = datastore.KeyForObj(ctx, e)
	}

	// A set of items we pretend not to see. Initially all tombstoned ones.
	//
	// Since we are iterating over tombstone list anyway, find all sufficiently
	// old tombstones or tombstones that still have storage associated with them.
	// We return them to the caller, so they can be cleaned up:
	//   * 'CleanupGarbage' makes sure 'storage' entities are deleted.
	//   * 'BeginPop' completely erases old tombstones.
	var tombs Garbage
	ignore := stringset.New(len(tombsEntity.Tombstones))
	for _, t := range tombsEntity.Tombstones {
		ignore.Add(t.ID)
		old := now.Sub(t.Tombstoned) > s.TombstonesDelay
		if storage, ok := idToKey[t.ID]; ok || old {
			tombs = append(tombs, &tombstone{
				id:      t.ID,
				storage: storage,
				old:     old, // if true, BeginPop will delete this tombstone
			})
		}
	}

	// Throw away tombstoned items.
	var items []Item
	for _, e := range entities {
		if !ignore.Has(e.ID) {
			items = append(items, Item{
				ID:    e.ID,
				Value: e.Value,
			})
			ignore.Add(e.ID)
		}
	}

	return &Listing{
		Items:      items,
		Garbage:    tombs,
		parent:     s.Parent,
		producedAt: now,
		idToKey:    idToKey,
	}, nil
}

// Delete deletes items from the set non-transactionally.
//
// Use at your own risk. If in doubt, use expected BeginPop() instead.
//
// Calls nextID() to get next ID to delete until nextID() returns "".
func (s *Set) Delete(ctx context.Context, nextID func() string) (err error) {
	if datastore.CurrentTransaction(ctx) != nil {
		return errors.New("dsset.Set.Delete must be called outside of a transaction")
	}
	ctx, span := tracing.Start(ctx, "go.chromium.org/luci/cv/internal/eventbox/dsset/Delete")
	defer func() { tracing.End(span, err) }()

	keys := []*datastore.Key{}
	for {
		id := nextID()
		if id == "" {
			break
		}
		keys = append(keys, datastore.NewKey(ctx, "dsset.Item", id, 0, s.Parent))
	}
	return transient.Tag.Apply(datastore.Delete(ctx, keys))
}

// PopOp is an in-progress 'Pop' operation.
//
// See BeginPop.
type PopOp struct {
	ctx      context.Context           // datastore context to use for this op
	txn      datastore.Transaction     // a transaction that started BeginPop
	now      time.Time                 // popping time for all popped items
	dirty    bool                      // true if the tombstone map was modified
	finished bool                      // true if finished already
	entity   *tombstonesEntity         // entity with tombstones
	tombs    map[string]time.Time      // entity.Tombstones in a map form
	idToKey  map[string]*datastore.Key // ID -> datastore key to cleanup
	popped   Garbage                   // new tombstones for popped items
}

// BeginPop initiates 'Pop' operation.
//
// Pop operation is used to transactionally remove items from the set, as well
// as cleanup old tombstones. It must be finished with 'dsset.FinishPop', even
// if no items have been popped: the internal state still can change in this
// case, since 'BeginPop' cleans up old tombstones. Even more, it is necessary
// to do 'Pop' if listing contains non-empty set of tombstones (regardless of
// whether the caller wants to actually pop any items from the set). This is
// part of the required set maintenance.
//
// Requires a transaction. Modifies Tombstone entity.
// Requires a txndefer to be installed in context. This is already done by
// default in luci/server.
//
// Returns only transient errors. Such errors usually mean that the entire pop
// sequence ('List' + 'Pop') should be retried.
func (s *Set) BeginPop(c context.Context, listing *Listing) (*PopOp, error) {
	if listing.parent != s.Parent {
		return nil, errors.New("passed Listing from another set")
	}
	txn := datastore.CurrentTransaction(c)
	if txn == nil {
		return nil, errors.New("dsset.Set.BeginPop must be called inside a transaction")
	}

	now := clock.Now(c).UTC()
	if age := now.Sub(listing.producedAt); age > s.TombstonesDelay {
		return nil, transient.Tag.Apply(fmt.Errorf("the listing is stale (%s > %s)", age, s.TombstonesDelay))
	}

	entity := &tombstonesEntity{Parent: s.Parent}
	if err := datastore.Get(c, entity); err != nil && err != datastore.ErrNoSuchEntity {
		return nil, transient.Tag.Apply(err)
	}

	// The data in tombstonesEntity, in map form.
	tombs := make(map[string]time.Time, len(entity.Tombstones))
	for _, t := range entity.Tombstones {
		tombs[t.ID] = t.Tombstoned
	}

	// Throw away old tombstones right away.
	dirty := false
	for _, tomb := range listing.Garbage {
		if tomb.old {
			if !tomb.cleanedUp {
				return nil, errors.New("trying to remove Tombstone that wasn't cleaned up")
			}
			if _, hasTomb := tombs[tomb.id]; hasTomb {
				delete(tombs, tomb.id)
				dirty = true
			}
		}
	}

	return &PopOp{
		ctx:     c,
		txn:     txn,
		now:     now,
		dirty:   dirty,
		entity:  entity,
		tombs:   tombs,
		idToKey: listing.idToKey,
	}, nil
}

// CanPop returns true if the given item can be popped from the set.
//
// Returns false if this item has been popped before (perhaps in another
// transaction), or it's not in the listing passed to BeginPop.
func (p *PopOp) CanPop(id string) bool {
	if _, hasTomb := p.tombs[id]; hasTomb {
		return false // already popped by someone else
	}
	if _, present := p.idToKey[id]; present {
		return true // listed in the set
	}
	return false
}

// Pop removed the item from the set and returns true if it was there.
//
// Returns false if this item has been popped before (perhaps in another
// transaction), or it's not in the listing passed to BeginPop.
func (p *PopOp) Pop(id string) bool {
	if p.finished {
		return false
	}
	if !p.CanPop(id) {
		return false
	}
	p.tombs[id] = p.now
	p.popped = append(p.popped, &tombstone{
		id:      id,
		storage: p.idToKey[id],
	})
	p.dirty = true
	return true
}

// makeTombstonesEntity is used internally by FinishPop.
func (p *PopOp) makeTombstonesEntity() *tombstonesEntity {
	p.entity.Tombstones = p.entity.Tombstones[:0]
	for id, ts := range p.tombs {
		p.entity.Tombstones = append(p.entity.Tombstones, struct {
			ID         string
			Tombstoned time.Time
		}{id, ts})
	}
	return p.entity
}

////////////////////////////////////////////////////////////////////////////////

// FinishPop completes one or more pop operations (for different sets) by
// submitting changes to datastore.
//
// Must be called within the same transaction that called BeginPop.
//
// Returns only transient errors.
func FinishPop(ctx context.Context, ops ...*PopOp) error {
	txn := datastore.CurrentTransaction(ctx)

	entities := []*tombstonesEntity{}
	tombsCount := 0
	for _, op := range ops {
		if op.finished {
			return errors.New("the operation has already been finished")
		}
		if op.txn != txn {
			return errors.New("wrong transaction")
		}
		if op.dirty {
			entities = append(entities, op.makeTombstonesEntity())
			tombsCount += len(op.popped)
		}
	}

	if err := datastore.Put(ctx, entities); err != nil {
		return transient.Tag.Apply(err)
	}

	var tombs Garbage
	if tombsCount != 0 {
		tombs = make(Garbage, 0, tombsCount)
	}
	for _, op := range ops {
		tombs = append(tombs, op.popped...)
		op.finished = true
	}
	txndefer.Defer(ctx, func(ctx context.Context) {
		CleanupGarbage(ctx, tombs) // best-effort cleanup
	})
	return nil
}

// CleanupGarbage deletes entities used to store items under given tombstones.
//
// This is datastore's MultiDelete RPC in disguise.
// Must be called outside of transactions. Idempotent.
//
// Can handle tombstones from multiple different sets at once. This is preferred
// over calling 'CleanupGarbage' multiple times (once per set), since it
// collapses multiple datastore RPCs into one.
//
// This MUST be called before tombstones returned by 'List' are removed in
// 'Pop'. Failure to do so will make items reappear in the set.
//
// Returns only transient errors. There's no way to know which items were
// removed and which weren't in case of an error.
func CleanupGarbage(ctx context.Context, cleanup ...Garbage) (err error) {
	if datastore.CurrentTransaction(ctx) != nil {
		return errors.New("dsset.CleanupGarbage must be called outside of a transaction")
	}
	ctx, span := tracing.Start(ctx, "go.chromium.org/luci/cv/internal/eventbox/dsset/CleanupGarbage")
	defer func() { tracing.End(span, err) }()

	keys := []*datastore.Key{}
	for _, tombs := range cleanup {
		for _, tomb := range tombs {
			if tomb.storage != nil {
				keys = append(keys, tomb.storage)
			}
		}
	}

	if err := datastore.Delete(ctx, keys); err != nil {
		return transient.Tag.Apply(err)
	}

	for _, tombs := range cleanup {
		for _, tomb := range tombs {
			tomb.cleanedUp = true
			tomb.storage = nil
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

type itemEntity struct {
	_kind string `gae:"$kind,dsset.Item"`

	ID     string         `gae:"$id"`
	Parent *datastore.Key `gae:"$parent"`
	Value  []byte         `gae:",noindex"`
}

type tombstonesEntity struct {
	_kind string `gae:"$kind,dsset.Tombstones"`

	ID     string         `gae:"$id,const"` // Always the same ID.
	Parent *datastore.Key `gae:"$parent"`

	// Tombstones is unordered list of pairs <item ID, when it was popped>.
	Tombstones []struct {
		ID         string
		Tombstoned time.Time
	} `gae:",noindex"`
}
