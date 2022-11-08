// Copyright 2022 The LUCI Authors.
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

package datastoreutil

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// CountLatestRevertsCreated returns the number of reverts created within
// the last number of hours
func CountLatestRevertsCreated(c context.Context, hours int64) (int64, error) {
	cutoffTime := clock.Now(c).Add(-time.Hour * time.Duration(hours))
	q := datastore.NewQuery("Suspect").
		Gt("revert_create_time", cutoffTime).
		Eq("is_revert_created", true)

	count, err := datastore.Count(c, q)
	if err != nil {
		err = errors.Annotate(err, "failed counting latest reverts created").Err()
		return 0, err
	}

	return count, nil
}

// CountLatestRevertsCommitted returns the number of reverts committed within
// the last number of hours
func CountLatestRevertsCommitted(c context.Context, hours int64) (int64, error) {
	cutoffTime := clock.Now(c).Add(-time.Hour * time.Duration(hours))
	q := datastore.NewQuery("Suspect").
		Gt("revert_commit_time", cutoffTime).
		Eq("is_revert_committed", true)

	count, err := datastore.Count(c, q)
	if err != nil {
		err = errors.Annotate(err, "failed counting latest reverts committed").Err()
		return 0, err
	}

	return count, nil
}
