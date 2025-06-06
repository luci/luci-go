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

package dscache

import (
	"context"
	"time"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	ds "go.chromium.org/luci/gae/service/datastore"
)

// internalValueSizeLimit is a var for testing purposes.
var internalValueSizeLimit = ValueSizeLimit

type dsCache struct {
	ds.RawInterface

	*supportContext
}

var _ ds.RawInterface = (*dsCache)(nil)

func (d *dsCache) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	return d.mutation(keys, func() error {
		return d.RawInterface.DeleteMulti(keys, cb)
	})
}

func (d *dsCache) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) error {
	return d.mutation(keys, func() error {
		return d.RawInterface.PutMulti(keys, vals, cb)
	})
}

func (d *dsCache) GetMulti(keys []*ds.Key, metas ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	rnd := mathrand.Get(d.c)

	itemKeys := d.mkRandKeys(keys, metas, rnd)
	if len(itemKeys) == 0 {
		return d.RawInterface.GetMulti(keys, metas, cb)
	}

	nonce := generateNonce(rnd)
	lockItems, err := d.impl.TryLockAndFetch(d.c, itemKeys, nonce, RefreshLockTimeout)
	if err != nil {
		logging.WithError(err).Debugf(d.c, "dscache: GetMulti: TryLockAndFetch")
	}

	p := d.makeFetchPlan(facts{keys, metas, lockItems, nonce})

	if !p.empty() {
		// looks like we have something to pull from datastore, and maybe some work
		// to save stuff back to memcache.

		var toCas []CacheItem
		err := d.RawInterface.GetMulti(p.toGet, p.toGetMeta, func(j int, pm ds.PropertyMap, err error) {
			i := p.idxMap[j]

			if err == nil {
				p.decoded[i] = pm
			} else {
				p.lme.Assign(i, err)
				if err != ds.ErrNoSuchEntity {
					return
				}
				pm = nil
			}

			toSave := p.toSave[j]
			if toSave == nil {
				return
			}

			expiry := time.Duration(ds.GetMetaDefault(
				metas.GetSingle(i),
				CacheExpirationMeta,
				int64(CacheDuration.Seconds()),
			).(int64)) * time.Second

			if pm == nil {
				// Missing entities are denoted by an empty data buffer.
				toSave.PromoteToData(toSave.Prefix(), expiry)
			} else {
				// Serialize and compress the PropertyMap, bail if too large.
				buf, err := encodeItemValue(pm, toSave.Prefix())
				if err == nil && len(buf) > internalValueSizeLimit {
					err = errors.Fmt("encoded entity too big (%d > %d)", len(buf), internalValueSizeLimit)
				}
				if err == nil {
					// The item should be able to fit into the cache.
					toSave.PromoteToData(buf, expiry)
				} else {
					// The item is "broken". No one else should try to serialize this item
					// until something Put/Delete's it. Set a lock on it.
					logging.WithError(err).Warningf(d.c, "dscache: PropertyMap serialization error")
					toSave.PromoteToIndefiniteLock()
				}
			}
			toCas = append(toCas, toSave)
		})

		if err != nil {
			// TODO(vadimsh): Should we drop locks owned by us?
			return err
		}

		if len(toCas) > 0 {
			// Store stuff we fetched back into memcache unless someone (like
			// a concurrent Put) deleted our locks already.
			if err := d.impl.CompareAndSwap(d.c, toCas); err != nil {
				logging.WithError(err).Debugf(d.c, "dscache: GetMulti: CompareAndSwap")
			}
		}
	}

	// Finally, run the callback for all of the decoded items and the errors,
	// if any.
	for i, dec := range p.decoded {
		cb(i, dec, p.lme.GetOne(i))
	}

	return nil
}

func (d *dsCache) RunInTransaction(f func(context.Context) error, opts *ds.TransactionOptions) error {
	txnState := dsTxnState{}
	err := d.RawInterface.RunInTransaction(func(ctx context.Context) error {
		txnState.reset()
		err := f(context.WithValue(ctx, &dsTxnCacheKey, &txnState))
		if err == nil {
			err = txnState.apply(d.supportContext)
		}
		return err
	}, opts)
	// Note: the transaction can *eventually* succeed even if `err` is non-nil
	// here. So on errors we pessimistically keep the locks until they expire.
	if err == nil {
		txnState.release(d.supportContext)
	}
	return err
}
