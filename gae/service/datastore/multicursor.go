// Copyright 2023 The LUCI Authors.
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

// Package datastore contains APIs to handle datastore queries
package datastore

import (
	"context"
	"sort"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	mc "go.chromium.org/luci/gae/service/datastore/internal/protos/multicursor"
)

// multiCursorVersion stores the proto version for mc.Cursors
const multiCursorVersion = 0

const multiCursorMagic = 0xA455

// IsMultiCursor returns true if the cursor probably represents a multicursor
// that is returned by RunMulti. Returns false otherwise
//
// Note: There is finite chance that some other cursor can be decoded as a valid
// multicursor
func IsMultiCursor(cursor RawCursor) bool {
	return IsMultiCursorString(cursor.String())
}

// IsMultiCursorString returns true if the cursor string is probably a valid
// representation of a multicursor that is returned by RunMulti. Returns false
// otherwise
//
// Note: There is finite chance that some other cursor can be decoded as a valid
// multicursor
func IsMultiCursorString(cursor string) bool {
	cursBuf, err := decodeBase64(cursor)
	if err != nil {
		// Cannot be a multicursor
		return false
	}
	var curs mc.Cursors
	err = proto.Unmarshal(cursBuf, &curs)
	return err == nil && curs.GetMagicNumber() == multiCursorMagic
}

// ApplyCursors applies the cursors to the queries and returns the new list of queries.
// The cursor should be from RunMulti, this will not work on any other cursor. The queries
// should match the original list of queries that was used to generate the cursor. If
// the queries don't match the behavior is undefined. The order for the queries is not
// important as they will be sorted before use.
func ApplyCursors(ctx context.Context, queries []*Query, cursor Cursor) ([]*Query, error) {
	if len(queries) != len(cursor) {
		return nil, errors.New("Length mismatch. Cannot apply this cursor to the queries")
	}
	// sortedOrder will contain the sorted order for queries. This allows
	// for updating the queries in order.
	sortedOrder := make([]int, len(queries))
	for idx := range sortedOrder {
		sortedOrder[idx] = idx
	}
	// Sort queries and store the order in sortedOrder
	sort.Slice(sortedOrder, func(i, j int) bool {
		return queries[sortedOrder[i]].Less(queries[sortedOrder[j]])
	})
	// Assign the cursors in sorted order
	for idx, qIdx := range sortedOrder {
		if cursor[idx] != nil {
			queries[qIdx] = queries[qIdx].Start(cursor[idx])
		}
	}
	// Return the queries in the order received
	return queries, nil
}

// ApplyCursorString applies the cursors represented by the string and returns the new
// list of queries. The cursor string should be generated from cursor returned by
// RunMulti, this will not work on any other cursor. The queries must match the original
// list of queries that was used to generate the cursor. If the queries don't match
// the behavior is undefined. The order of queries is not important as they will be
// sorted before use.
func ApplyCursorString(ctx context.Context, queries []*Query, cursorToken string) ([]*Query, error) {
	cursor, err := DecodeCursor(ctx, cursorToken)
	if err != nil {
		return nil, errors.Fmt("Failed to decode cursor: %w", err)
	}
	return ApplyCursors(ctx, queries, cursor)
}
