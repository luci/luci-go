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
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
)

const (
	// BeginingOfTheWorld is a unix time for 2010-01-01 00:00:00 +0000 UTC.
	BeginingOfTheWorld = int64(1262304000)

	timeResolution   = time.Millisecond * 1
	buildIDSuffixLen = 20
)

// GetIgnoreMissing fetches the given entities from the datastore,
// ignoring datastore.ErrNoSuchEntity errors. All other errors are returned.
// For valid values of dst, see datastore.Get.
func GetIgnoreMissing(ctx context.Context, dst ...interface{}) error {
	if err := datastore.Get(ctx, dst...); err != nil {
		merr, ok := err.(errors.MultiError)
		if !ok {
			return err
		}
		for _, e := range merr {
			if e != nil && e != datastore.ErrNoSuchEntity {
				return err
			}
		}
	}
	return nil
}

// BuildIDRange converts a creation time range to build id range.
// Low/high bounds are inclusive/exclusive respectively
// for both time and id ranges
func BuildIDRange(timeLow time.Time, timeHigh time.Time) (int64, int64) {
	// convert inclusive to exclusive
	idHigh := idTimeSegment(timeLow.Add(-timeResolution))
	// convert exclusive to inclusive
	idLow := idTimeSegment(timeHigh.Add(-timeResolution))
	return idLow, idHigh
}

func idTimeSegment(dtime time.Time) int64 {
	delta := dtime.Sub(time.Unix(BeginingOfTheWorld, 0)).Milliseconds()
	if delta < 0 {
		return 0
	}
	return (^delta & ((1 << 43) - 1)) << 20
}
