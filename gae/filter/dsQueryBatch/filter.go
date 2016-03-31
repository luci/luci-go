// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dsQueryBatch

import (
	"fmt"

	ds "github.com/luci/gae/service/datastore"
)

type iterQueryFilter struct {
	ds.RawInterface

	batchSize int32
}

func (f *iterQueryFilter) Run(fq *ds.FinalizedQuery, cb ds.RawRunCB) error {
	limit, hasLimit := fq.Limit()

	var cursor ds.Cursor
	for {
		iterQuery := fq.Original()
		if cursor != nil {
			iterQuery = iterQuery.Start(cursor)
			cursor = nil
		}
		iterLimit := f.batchSize
		if hasLimit && limit < iterLimit {
			iterLimit = limit
		}
		iterQuery = iterQuery.Limit(iterLimit)

		iterFinalizedQuery, err := iterQuery.Finalize()
		if err != nil {
			panic(fmt.Errorf("failed to finalize internal query: %v", err))
		}

		count := int32(0)
		err = f.RawInterface.Run(iterFinalizedQuery, func(key *ds.Key, val ds.PropertyMap, getCursor ds.CursorCB) error {
			if cursor != nil {
				// We're iterating past our batch size, which should never happen, since
				// we set a limit. This will only happen when our inner RawInterface
				// fails to honor the limit that we set.
				panic(fmt.Errorf("iterating past batch size"))
			}

			if err := cb(key, val, getCursor); err != nil {
				return err
			}

			// If this is the last entry in our batch, get the cursor and stop this
			// query round.
			count++
			if count >= f.batchSize {
				if cursor, err = getCursor(); err != nil {
					return fmt.Errorf("failed to get cursor: %v", err)
				}
			}
			return nil
		})
		if err != nil && err != ds.Stop {
			return err
		}

		// If we have no cursor, we're done.
		if cursor == nil {
			break
		}

		// Reduce our limit for the next round.
		if hasLimit {
			limit -= count
			if limit <= 0 {
				break
			}
		}
	}
	return nil
}
