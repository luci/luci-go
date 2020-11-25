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
	"sync"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"

	ds "go.chromium.org/luci/gae/service/datastore"
)

type dsTxnState struct {
	sync.Mutex

	toLock stringset.Set // keys touched in the current txn attempt
	toDrop stringset.Set // keys ever locked (across all attempts)
}

// reset sets the transaction state back to its 0 state. This is used so that
// when a transaction retries the function, we don't accidentally leak state
// from one function to the next.
func (s *dsTxnState) reset() {
	s.Lock()
	defer s.Unlock()

	s.toLock = stringset.New(s.toLock.Len())
}

// apply is called right before the transaction is about to commit. It's job
// is to lock all the to-be-changed memcache keys.
func (s *dsTxnState) apply(sc *supportContext) error {
	s.Lock()
	defer s.Unlock()

	if s.toLock.Len() != 0 {
		// We are about to lock some keys. Need to drop them later.
		s.toDrop = s.toDrop.Union(s.toLock)
		// This is a hard failure. No mutation can occur if we're unable to set
		// locks out. See "DANGER ZONE" in the docs.
		if err := sc.impl.PutLocks(sc.c, s.toLock.ToSlice(), MutationLockTimeout); err != nil {
			logging.WithError(err).Errorf(sc.c, "dscache: HARD FAILURE: dsTxnState.apply(): PutLocks")
			return err
		}
	}

	return nil
}

// release is called right after a successful transaction completion. It's job
// is to clear out all the locks, if possible (but if not, no worries,
// they'll expire soon).
func (s *dsTxnState) release(sc *supportContext) {
	s.Lock()
	defer s.Unlock()

	if err := sc.impl.DropLocks(sc.c, s.toDrop.ToSlice()); err != nil {
		logging.WithError(err).Warningf(sc.c, "dscache: dsTxnState.release: DropLocks")
	}
}

func (s *dsTxnState) add(sc *supportContext, keys []*ds.Key) {
	if itemKeys := sc.mkAllKeys(keys); len(itemKeys) != 0 {
		s.Lock()
		defer s.Unlock()
		s.toLock.AddAll(itemKeys)
	}
}
