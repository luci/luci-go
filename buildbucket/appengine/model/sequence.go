// Copyright 2020 The LUCI Authors.
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

package model

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// NumberSequence stores the next number in a named sequence of numbers.
type NumberSequence struct {
	_     datastore.PropertyMap `gae:"-,extra"`
	_kind string                `gae:"$kind,NumberSequence"`
	ID    string                `gae:"$id"`

	Next int32 `gae:"next_number,noindex"`
}

// GenerateSequenceNumbers generates n numbers for the given sequence name,
// returning the smallest number the caller may use. For a returned number
// i, the caller may use [i, i+n).
func GenerateSequenceNumbers(ctx context.Context, name string, n int) (int32, error) {
	// TODO(crbug/1042991): Migrate old -> new build number sequence entity key.
	// TODO(crbug/1042991): Report metric for sequence number generation time.
	seq := &NumberSequence{
		ID: name,
	}
	var res int32
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		switch err := datastore.Get(ctx, seq); {
		case err == datastore.ErrNoSuchEntity:
			seq.Next = 1
		case err != nil:
			return errors.Fmt("error fetching number sequence %q: %w", seq.ID, err)
		}
		res = seq.Next
		seq.Next += int32(n)
		if err := datastore.Put(ctx, seq); err != nil {
			return errors.Fmt("error updating number sequence %q: %w", seq.ID, err)
		}
		return nil
	}, nil)
	if err != nil {
		return 0, err
	}
	return res, nil
}
