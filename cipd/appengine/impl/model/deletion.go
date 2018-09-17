// Copyright 2018 The LUCI Authors.
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
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
)

var (
	// List of entity kinds to delete when removing a package. We purposefully
	// do not delete the entire entity group (using kindless datastore query) to
	// make sure that whoever adds more entity types there is cognizant of the
	// deletion procedure.
	kindsToDelete = []string{
		"InstanceTag",
		"PackageInstance",
		"PackageRef",
		"ProcessingResult",
	}
	// Number of keys to delete at once in DeletePackage. Replaced in tests.
	deletionBatchSize = 256
)

// DeletePackage deletes all entities associated with a package.
//
// Returns grpc-tagged errors (in particular NotFound if there's no such
// package).
func DeletePackage(c context.Context, pkg string) error {
	// TODO(vadimsh): This transaction can become too big for packages with a lot
	// of instances. If this becomes a problem, we can delete most of the stuff
	// non-transactionally first, and then delete the rest inside the transaction
	// (to make sure nothing stays, even if it was created while we were deleting
	// stuff).
	return Txn(c, "DeletePackage", func(c context.Context) error {
		if err := CheckPackageExists(c, pkg); err != nil {
			return err
		}

		err := parallel.WorkPool(len(kindsToDelete)+1, func(tasks chan<- func() error) {
			// A channel that receives keys to delete. Set some arbitrary buffer
			// size to parallelize work a bit better.
			keys := make(chan *datastore.Key, 1024)

			// Launch queries that fetch keys to delete, and feed them to the channel.
			// Each query enqueues nil when it is done to let the consumer know.
			for _, kind := range kindsToDelete {
				kind := kind
				tasks <- func() error {
					q := datastore.NewQuery(kind).
						Ancestor(PackageKey(c, pkg)).
						KeysOnly(true)

					count := 0
					err := datastore.Run(c, q, func(k *datastore.Key, _ datastore.CursorCB) error {
						count += 1
						keys <- k
						return nil
					})

					keys <- nil // put "we are done" signal
					if err == nil {
						logging.Infof(c, "Found %d %q entities to be deleted", count, kind)
					} else {
						logging.WithError(err).Errorf(c, "Found %d %q entities and then failed", count, kind)
					}
					return err
				}
			}

			// Delete keys in batches. Stop when we get all "we are done" signals from
			// all len(kindsToDelete) queries. Carry on on errors, to make sure to
			// drain the channel completely (otherwise enqueue ops will get blocked).
			tasks <- func() error {
				var batch []*datastore.Key
				var errs errors.MultiError

				// flush deletes all keys recorded in 'batch'.
				flush := func() {
					logging.Infof(c, "Deleting %d entities...", len(batch))
					if err := datastore.Delete(c, batch); err != nil {
						logging.WithError(err).Errorf(c, "Failed to delete %d entities", len(batch))
						errs = append(errs, err)
					}
					batch = batch[:0]
				}

				stillRunning := len(kindsToDelete)
				for k := range keys {
					if k != nil {
						if batch = append(batch, k); len(batch) >= deletionBatchSize {
							flush()
						}
						continue
					}

					// Got "we are done" signal from some query.
					if stillRunning -= 1; stillRunning == 0 {
						break // all queries are done
					}
				}

				// Flush the last incomplete batch.
				if len(batch) > 0 {
					flush()
				}
				if len(errs) != 0 {
					return errs
				}
				return nil
			}
		})
		if err != nil {
			return transient.Tag.Apply(err)
		}

		// Finally, delete the package entity itself. Its absence is a marker that
		// the package is completely gone (as checked in CheckPackageExists).
		return datastore.Delete(c, PackageKey(c, pkg))
	})
}
