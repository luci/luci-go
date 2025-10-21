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
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/common"
)

// Tag represents a "key:value" pair attached to some package instance.
//
// Tags exist as separate entities (as opposed to being a repeated property
// of Instance entity) to allow essentially unlimited number of them. Also we
// often do not want to fetch all tags when fetching Instance entity.
//
// ID is hex-encoded SHA1 of the tag. The parent entity is the corresponding
// Instance. The tag string can't be made a part of the key because the total
// key length (including entity kind names and all parent keys) is limited to
// 500 bytes and tags are allowed to be pretty long (up to 400 bytes).
//
// There's a possibility someone tries to abuse a collision in SHA1 hash
// used by TagID() to attach or detach some wrong tag. The chance is minuscule,
// and it's not a big deal if it happens now, but it may become important in the
// future once tags have their own ACLs. For the sake of paranoia, we double
// check 'Tag' field in all entities we touch.
//
// Compatible with the python version of the backend.
type Tag struct {
	_kind  string                `gae:"$kind,InstanceTag"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	ID       string         `gae:"$id"`     // see TagID()
	Instance *datastore.Key `gae:"$parent"` // key of corresponding Instance entity

	Tag          string    `gae:"tag"`           // the tag itself, as "k:v" pair
	RegisteredBy string    `gae:"registered_by"` // who added this tag
	RegisteredTs time.Time `gae:"registered_ts"` // when it was added
}

// Proto returns cipd.Tag proto with information from this entity.
//
// Assumes the tag is valid.
func (t *Tag) Proto() *repopb.Tag {
	kv := strings.SplitN(t.Tag, ":", 2)
	if len(kv) != 2 {
		panic(fmt.Sprintf("bad tag %q", t.Tag))
	}
	return &repopb.Tag{
		Key:        kv[0],
		Value:      kv[1],
		AttachedBy: t.RegisteredBy,
		AttachedTs: timestamppb.New(t.RegisteredTs),
	}
}

// TagID calculates Tag entity ID (SHA1 digest) from the given tag.
//
// Panics if the tag is invalid.
func TagID(t *repopb.Tag) string {
	tag := common.JoinInstanceTag(t)
	if err := common.ValidateInstanceTag(tag); err != nil {
		panic(err)
	}
	digest := sha1.Sum([]byte(tag))
	return hex.EncodeToString(digest[:])
}

// AttachTags transactionally attaches a bunch of tags to an instance.
//
// Assumes inputs are already validated. Launches a transaction inside (and thus
// can't be a part of a transaction itself). Updates 'inst' in-place with the
// most recent instance state.
//
// Returns gRPC-tagged errors:
//
//	NotFound if there's no such instance or package.
//	FailedPrecondition if some processors are still running.
//	Aborted if some processors have failed.
//	Internal on tag ID collision.
func AttachTags(ctx context.Context, inst *Instance, tags []*repopb.Tag) error {
	return Txn(ctx, "AttachTags", func(ctx context.Context) error {
		if err := CheckInstanceReady(ctx, inst); err != nil {
			return err
		}

		_, missing, err := checkExistingTags(ctx, inst, tags)
		if err != nil {
			return err
		}

		events := Events{}
		now := clock.Now(ctx).UTC()
		who := string(auth.CurrentIdentity(ctx))
		for _, t := range missing {
			t.RegisteredBy = who
			t.RegisteredTs = now
			events.Emit(&repopb.Event{
				Kind:     repopb.EventKind_INSTANCE_TAG_ATTACHED,
				Package:  inst.Package.StringID(),
				Instance: inst.InstanceID,
				Tag:      t.Tag,
				Who:      who,
				When:     timestamppb.New(now),
			})
		}

		if err := datastore.Put(ctx, missing); err != nil {
			return transient.Tag.Apply(err)
		}
		return events.Flush(ctx)
	})
}

// DetachTags detaches a bunch of tags from an instance.
//
// Assumes inputs are already validated. Launches a transaction inside (and thus
// can't be a part of a transaction itself).
//
// 'inst' is used only for its key.
func DetachTags(ctx context.Context, inst *Instance, tags []*repopb.Tag) error {
	return Txn(ctx, "DetachTags", func(ctx context.Context) error {
		existing, _, err := checkExistingTags(ctx, inst, tags)
		if err != nil {
			return err
		}

		if err := datastore.Delete(ctx, existing); err != nil {
			return transient.Tag.Apply(err)
		}

		events := Events{}
		for _, t := range existing {
			events.Emit(&repopb.Event{
				Kind:     repopb.EventKind_INSTANCE_TAG_DETACHED,
				Package:  inst.Package.StringID(),
				Instance: inst.InstanceID,
				Tag:      t.Tag,
			})
		}
		return events.Flush(ctx)
	})
}

// ResolveTag searches for a given tag among all instances of the package and
// returns an ID of the instance the tag is attached to.
//
// Assumes inputs are already validated. Doesn't double check the instance
// exists.
//
// Returns gRPC-tagged errors:
//
//	NotFound if there's no such tag at all.
//	FailedPrecondition if the tag resolves to multiple instances.
func ResolveTag(ctx context.Context, pkg string, tag *repopb.Tag) (string, error) {
	// TODO(vadimsh): Cache the result of the resolution. This is generally not
	// trivial to do right without race conditions, preserving the consistency
	// guarantees of the API. This will become simpler once we have a notion of
	// unique tags.
	q := datastore.NewQuery("InstanceTag").
		Ancestor(PackageKey(ctx, pkg)).
		Eq("tag", common.JoinInstanceTag(tag)).
		KeysOnly(true).
		Limit(2)

	var tags []*Tag
	switch err := datastore.GetAll(ctx, q, &tags); {
	case err != nil:
		return "", transient.Tag.Apply(errors.Fmt("failed to query tags: %w", err))
	case len(tags) == 0:
		return "", grpcutil.NotFoundTag.Apply(errors.New("no such tag"))
	case len(tags) > 1:
		return "", grpcutil.FailedPreconditionTag.Apply(errors.New("ambiguity when resolving the tag, more than one instance has it"))
	default:
		return tags[0].Instance.StringID(), nil
	}
}

// ListInstanceTags returns all tags attached to an instance, sorting them by
// the tag key first, and then by the timestamp (most recent first).
//
// Returns an empty list if there's no such instance at all.
func ListInstanceTags(ctx context.Context, inst *Instance) (out []*Tag, err error) {
	// TODO(vadimsh): Sorting by tag key requires adding the key as a field in the
	// entity and setting up a composite index (key, -registered_ts). This change
	// requires a migration of existing entities. We punt on this for now and sort
	// in memory instead (just like we did on the client before). The most heavily
	// tagged instances in our datastore have few hundred tags at most, so this is
	// bearable.
	q := datastore.NewQuery("InstanceTag").Ancestor(datastore.KeyForObj(ctx, inst))
	if datastore.GetAll(ctx, q, &out); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("datastore query failed: %w", err))
	}
	sort.Slice(out, func(i, j int) bool {
		if k1, k2 := tagKey(out[i].Tag), tagKey(out[j].Tag); k1 != k2 {
			return k1 < k2
		}
		if !out[i].RegisteredTs.Equal(out[j].RegisteredTs) {
			return out[i].RegisteredTs.After(out[j].RegisteredTs)
		}
		return out[i].Tag < out[j].Tag
	})
	return
}

// tagKey takes "key:<stuff>" and returns "key".
//
// It is a faster version of common.ParseInstanceTags that skips validation
// (since stored entities are already validated).
//
// We micro-optimized this part because comparator in ListInstanceTags is a hot
// spot when sorting a lot of entities.
func tagKey(kv string) string {
	if idx := strings.IndexRune(kv, ':'); idx != -1 {
		return kv[:idx]
	}
	return kv
}

// checkExistingTags takes a list of tags and returns the ones that exist
// in the datastore and ones that don't.
//
// Checks 'Tag' field on existing tags, thus safeguarding against malicious
// SHA1 collisions in TagID().
//
// Tags in 'miss' list have only their key and 'Tag' fields set and nothing
// more.
func checkExistingTags(ctx context.Context, inst *Instance, tags []*repopb.Tag) (exist, miss []*Tag, err error) {
	instKey := datastore.KeyForObj(ctx, inst)
	tagEnts := make([]*Tag, len(tags))
	for i, tag := range tags {
		tagEnts[i] = &Tag{
			ID:       TagID(tag),
			Instance: instKey,
		}
	}
	return fetchTags(ctx, tagEnts, func(i int) *repopb.Tag { return tags[i] })
}

// fetchTags fetches given tag entities and categorized them into existing and
// missing ones.
//
// The entities doesn't have to be in same entity group.
//
// Checks 'Tag' field on existing tags (by comparing it to what 'expectedTag'
// callback returns for the corresponding index), thus safeguarding against
// malicious SHA1 collisions in TagID().
func fetchTags(ctx context.Context, tagEnts []*Tag, expectedTag func(idx int) *repopb.Tag) (exist, miss []*Tag, err error) {
	// Try to grab all entities and bail on unexpected errors.
	existCount := 0
	missCount := 0
	if err := datastore.Get(ctx, tagEnts); err != nil {
		merr, ok := err.(errors.MultiError)
		if !ok {
			return nil, nil, transient.Tag.Apply(errors.Fmt("failed to fetch tags: %w", err))
		}
		for i, err := range merr {
			switch err {
			case nil:
				existCount++
			case datastore.ErrNoSuchEntity:
				missCount++
			default:
				return nil, nil, transient.Tag.Apply(errors.Fmt("failed to fetch tag with ID %q: %w", tagEnts[i].ID, err))
			}
		}
	} else {
		existCount = len(tagEnts)
	}

	if existCount != 0 {
		exist = make([]*Tag, 0, existCount)
	}
	if missCount != 0 {
		miss = make([]*Tag, 0, missCount)
	}

	for i, ent := range tagEnts {
		switch kv := common.JoinInstanceTag(expectedTag(i)); {
		case ent.Tag == kv:
			exist = append(exist, ent)
		case ent.Tag == "":
			ent.Tag = kv // so the caller knows what tag this is
			miss = append(miss, ent)
		default: // ent.Tag != kv
			return nil, nil,
				grpcutil.InternalTag.Apply(errors.Fmt("tag %q collides with tag %q, refusing to touch it", kv, ent.Tag))
		}
	}

	return
}
