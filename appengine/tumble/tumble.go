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

// EnterTransaction is the method to use when doing transactional operations
// in the tumble ecosystem. It allows work to be done in the entity group
// specified by `fromRoot`, and any returned Mutation objects will be
// transactionally queued for the tumble backend.
//
// Usually this is called from your application's handlers to begin a tumble
// state machine as a result of some API interaction.
func EnterTransaction(c context.Context, fromRoot *datastore.Key, f func(context.Context) ([]Mutation, error)) error {
	shardSet, _, err := enterTransactionInternal(txnBuf.FilterRDS(c), fromRoot, f)
	if err != nil {
		return err
	}
	fireTasks(c, shardSet)
	return nil
}

func enterTransactionInternal(c context.Context, fromRoot *datastore.Key, f func(context.Context) ([]Mutation, error)) (map[uint64]struct{}, uint, error) {
	if fromRoot == nil {
		return nil, 0, fmt.Errorf("tumble: Passing nil as fromRoot is illegal")
	}

	shardSet := map[uint64]struct{}(nil)
	numNewMuts := uint(0)

	err := datastore.Get(c).RunInTransaction(func(c context.Context) error {
		// do a Get on the fromRoot to ensure that this transaction is associated
		// with that entity group.
		_, _ = datastore.Get(c).Exists(fromRoot)

		muts, err := f(c)
		if err != nil {
			return err
		}

		numNewMuts = uint(len(muts))
		shardSet, err = putMutations(c, fromRoot, muts)

		return err
	}, nil)
	if err != nil {
		return nil, 0, err
	}

	return shardSet, numNewMuts, nil
}
