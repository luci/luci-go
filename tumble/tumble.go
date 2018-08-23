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

package tumble

import (
	"fmt"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/gae/filter/txnBuf"
	ds "go.chromium.org/gae/service/datastore"

	"golang.org/x/net/context"
)

// RunMutation immediately runs the Mutation `m` in a transaction. This method
// should be used to start a tumble chain when you have transactional checks
// to do (e.g. `m` implements the first transactional link in the chain).
//
// Usually this is called from your application's handlers to begin a tumble
// state machine as a result of some API interaction.
func RunMutation(c context.Context, m Mutation) error {
	cfg := getConfig(c)
	shardSet, _, _, err := enterTransactionMutation(txnBuf.FilterRDS(c), cfg, m, 0)
	if err != nil {
		return err
	}
	fireTasks(c, cfg, shardSet, true)
	return nil
}

// RunUnbuffered opens a lightweight unbuffered transaction on "root"
// runs "fn" inside of it. Any mutations returned by "fn" will be registered
// at the end of the transaction if "fn" doesn't return an error.
//
// This is useful as an initial starting point without incurring any of the
// overhead of spinning up a new buffered transaction.
//
// During "fn"'s execution, standard Tumble operations such as PutNamedMutation
// and CancelNamedMutation may be performed.
func RunUnbuffered(c context.Context, root *ds.Key, fn func(context.Context) ([]Mutation, error)) error {
	cfg := getConfig(c)
	shardSet, _, _, err := enterTransactionInternal(c, cfg, root, fn, 0)
	if err != nil {
		return err
	}
	fireTasks(c, cfg, shardSet, true)
	return nil
}

func enterTransactionMutation(c context.Context, cfg *Config, m Mutation, round uint64) (
	map[taskShard]struct{}, []Mutation, []*ds.Key, error) {

	return enterTransactionInternal(c, cfg, m.Root(c), m.RollForward, round)
}

func enterTransactionInternal(c context.Context, cfg *Config, root *ds.Key, fn func(context.Context) ([]Mutation, error), round uint64) (
	map[taskShard]struct{}, []Mutation, []*ds.Key, error) {

	if root == nil {
		return nil, nil, nil, fmt.Errorf("tumble: Passing nil as root is illegal")
	}

	shardSet := map[taskShard]struct{}(nil)
	retMuts := []Mutation(nil)
	retMutKeys := []*ds.Key(nil)

	err := ds.RunInTransaction(c, func(c context.Context) error {
		// do a Get on the root to ensure that this transaction is associated
		// with that entity group.
		_, _ = ds.Exists(c, root)

		muts, err := fn(c)
		if err != nil {
			return err
		}

		retMuts = muts
		shardSet, retMutKeys, err = putMutations(c, cfg, root, muts, round)

		return err
	}, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	return shardSet, retMuts, retMutKeys, nil
}

// PutNamedMutations writes the provided named mutations to the datastore.
//
// Named mutations are singletons children of a given `parent`. So for any given
// `parent`, there can be only one mutation with any given name.  Named
// mutations, by design, cannot collide with unnamed (anonymous) mutations.
// `parent` does NOT need to be a root Key; the mutations will be created as
// direct descendants of the provided parent entity. The names of the various
// mutations should be valid datastore key StringIDs, which generally means that
// they should be UTF-8 strings.
//
// The current implementation reserves 2 characters of the StringID (as of
// writing this means that a named mutation name may only be 498 bytes long).
//
// This can be used to leverage tumble for e.g. cancellable, delayed cleanup
// tasks (like timeouts).
//
// This may be called from within an existing datastore transaction which
// includes `parent` to make this Put atomic with the remainder of the
// transaction.
//
// If called multiple times with the same name, the newly named mutation will
// overwrite the existing mutation (assuming it hasn't run already).
func PutNamedMutations(c context.Context, parent *ds.Key, muts map[string]Mutation) error {
	cfg := getConfig(c)

	now := clock.Now(c).UTC()

	shardSet := map[taskShard]struct{}{}
	toPut := make([]*realMutation, 0, len(muts))
	for name, m := range muts {
		realMut, err := newRealMutation(c, cfg, "n:"+name, parent, m, now)
		if err != nil {
			logging.WithError(err).Errorf(c, "error creating real mutation for %v", m)
			return err
		}
		toPut = append(toPut, realMut)
		shardSet[realMut.shard(cfg)] = struct{}{}
	}

	err := ds.Put(c, toPut)
	if err == nil {
		metricCreated.Add(c, int64(len(toPut)), parent.Namespace())
	}
	fireTasks(c, getConfig(c), shardSet, true)
	return err
}

// CancelNamedMutations does a best-effort cancellation of the named mutations.
func CancelNamedMutations(c context.Context, parent *ds.Key, names ...string) error {
	toDel := make([]*ds.Key, 0, len(names))
	nameSet := stringset.NewFromSlice(names...)
	nameSet.Iter(func(name string) bool {
		toDel = append(toDel, ds.NewKey(c, "tumble.Mutation", "n:"+name, 0, parent))
		return true
	})
	err := ds.Delete(c, toDel)
	numFailed := int64(0)
	if me, ok := err.(errors.MultiError); ok {
		numFailed = int64(len(me))
	}
	metricDeleted.Add(c, int64(len(toDel))-numFailed, parent.Namespace())
	return errors.Filter(err, ds.ErrNoSuchEntity)
}
