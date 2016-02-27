// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"fmt"

	"github.com/luci/gae/filter/txnBuf"
	"github.com/luci/gae/service/datastore"
	"golang.org/x/net/context"
)

// RunMutation is the method to use when doing transactional operations
// in the tumble ecosystem. It allows work to be done in the entity group
// specified by `fromRoot`, and any returned Mutation objects will be
// transactionally queued for the tumble backend.
//
// Usually this is called from your application's handlers to begin a tumble
// state machine as a result of some API interaction.
func RunMutation(c context.Context, m Mutation) error {
	cfg := getConfig(c)
	shardSet, _, _, err := enterTransactionInternal(txnBuf.FilterRDS(c), cfg, m, 0)
	if err != nil {
		return err
	}
	fireTasks(c, cfg, shardSet)
	return nil
}

func enterTransactionInternal(c context.Context, cfg *Config, m Mutation, round uint64) (map[taskShard]struct{}, []Mutation, []*datastore.Key, error) {
	fromRoot := m.Root(c)

	if fromRoot == nil {
		return nil, nil, nil, fmt.Errorf("tumble: Passing nil as fromRoot is illegal")
	}

	shardSet := map[taskShard]struct{}(nil)
	retMuts := []Mutation(nil)
	retMutKeys := []*datastore.Key(nil)

	err := datastore.Get(c).RunInTransaction(func(c context.Context) error {
		// do a Get on the fromRoot to ensure that this transaction is associated
		// with that entity group.
		_, _ = datastore.Get(c).Exists(fromRoot)

		muts, err := m.RollForward(c)
		if err != nil {
			return err
		}

		retMuts = muts
		shardSet, retMutKeys, err = putMutations(c, cfg, fromRoot, muts, round)

		return err
	}, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	return shardSet, retMuts, retMutKeys, nil
}
