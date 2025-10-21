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
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
)

func TestDeletePackage(t *testing.T) {
	t.Parallel()

	// No other test is hiting DeletePackage, so its fine to change the global
	// variable here in this parallel test.
	deletionBatchSize = 7

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		// Returns number of entities in an entity group, skipping unimportant ones.
		entitiesCount := func(root *datastore.Key) (count int64) {
			q := datastore.NewQuery("").Ancestor(root).KeysOnly(true)
			assert.Loosely(t, datastore.Run(ctx, q, func(k *datastore.Key) {
				switch {
				// Skip magical __entity_group__ entities created by the datastore.
				// This is roughly same as using GetTestable(ctx).DisableSpecialEntities
				// except we can't disable them, since we need them for transactions to
				// work.
				case strings.HasPrefix(k.Kind(), "__"):
					return
				// Event log is never deleted. Its entities reside in same entity group
				// as the rest of CIPD model.
				case k.Kind() == "Event":
					return
				default:
					count++
				}
			}), should.BeNil)
			return
		}

		// This will create 4*X entities to be deleted. For deletionBatchSize == 7
		// set above, it means we'll have some number of full batches and one
		// incomplete final batch, thus covering all important code paths.
		for _, chr := range []string{"a", "b", "c", "d"} {
			reg, inst, _ := RegisterInstance(ctx, &Instance{
				InstanceID: strings.Repeat(chr, 40),
				Package:    PackageKey(ctx, "pkg"),
			}, nil)
			assert.Loosely(t, reg, should.BeTrue)

			assert.Loosely(t, datastore.Put(ctx, &ProcessingResult{
				ProcID:   "zzz",
				Instance: datastore.KeyForObj(ctx, inst),
			}), should.BeNil)

			assert.Loosely(t, SetRef(ctx, chr+"-ref", inst), should.BeNil)
			assert.Loosely(t, AttachTags(ctx, inst, []*repopb.Tag{
				{Key: "k1", Value: chr},
				{Key: "k2", Value: chr},
			}), should.BeNil)
			assert.Loosely(t, AttachMetadata(ctx, inst, []*repopb.InstanceMetadata{
				{Key: "k1", Value: []byte(chr)},
				{Key: "k2", Value: []byte(chr)},
			}), should.BeNil)
		}

		// Some unrelated instance in a different package to be left alone.
		reg, _, _ := RegisterInstance(ctx, &Instance{
			InstanceID: strings.Repeat("a", 40),
			Package:    PackageKey(ctx, "another-pkg"),
		}, nil)
		assert.Loosely(t, reg, should.BeTrue)

		// Before the deletion.
		assert.Loosely(t, entitiesCount(PackageKey(ctx, "pkg")), should.Equal(29))
		assert.Loosely(t, entitiesCount(PackageKey(ctx, "another-pkg")), should.Equal(2))

		assert.Loosely(t, DeletePackage(ctx, "pkg"), should.BeNil)

		// After the deletion we are left only with another-pkg.
		assert.Loosely(t, entitiesCount(PackageKey(ctx, "pkg")), should.BeZero)
		assert.Loosely(t, entitiesCount(PackageKey(ctx, "another-pkg")), should.Equal(2))

		// Have the event in the log as well.
		events := GetEvents(ctx)
		assert.Loosely(t, events[len(events)-1], should.Match(&repopb.Event{
			Kind:    repopb.EventKind_PACKAGE_DELETED,
			Package: "pkg",
			Who:     string(testutil.TestUser),
			When:    timestamppb.New(testutil.TestTime),
		}))

		// And DeletePackage now complains that the package is gone.
		err := DeletePackage(ctx, "pkg")
		assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))

		// And it didn't generate any new events.
		assert.Loosely(t, GetEvents(ctx), should.HaveLength(len(events)))
	})
}
