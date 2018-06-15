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
	"crypto/sha1"
	"encoding/hex"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
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

// TagID calculates Tag entity ID (SHA1 digest) from the given tag.
//
// Panics if the tag is invalid.
func TagID(t *api.Tag) string {
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
//    NotFound if there's no such instance or package.
//    FailedPrecondition if some processors are still running.
//    Aborted if some processors have failed.
//    Internal on tag ID collision.
func AttachTags(c context.Context, inst *Instance, tags []*api.Tag, who identity.Identity) error {
	return Txn(c, "AttachTags", func(c context.Context) error {
		if err := CheckInstanceReady(c, inst); err != nil {
			return err
		}
		_, missing, err := checkExistingTags(c, inst, tags)
		if err != nil {
			return err
		}
		now := clock.Now(c).UTC()
		for _, t := range missing {
			t.RegisteredBy = string(who)
			t.RegisteredTs = now
		}
		return transient.Tag.Apply(datastore.Put(c, missing))
	})
}

// DetachTags detaches a bunch of tags from an instance.
//
// Assumes inputs are already validated. Launches a transaction inside (and thus
// can't be a part of a transaction itself).
//
// 'inst' is used only for its key.
func DetachTags(c context.Context, inst *Instance, tags []*api.Tag) error {
	return Txn(c, "DetachTags", func(c context.Context) error {
		existing, _, err := checkExistingTags(c, inst, tags)
		if err != nil {
			return err
		}
		return transient.Tag.Apply(datastore.Delete(c, existing))
	})
}

// ResolveTag searches for a given tag among all instances of the package and
// returns an ID of the instance the tag is attached to.
//
// Assumes inputs are already validated. Doesn't double check the instance
// exists.
//
// Returns gRPC-tagged errors:
//   NotFound if there's no such tag at all.
//   FailedPrecondition if the tag resolves to multiple instances.
func ResolveTag(c context.Context, pkg string, tag *api.Tag) (string, error) {
	// TODO(vadimsh): Cache the result of the resolution. This is generally not
	// trivial to do right without race conditions, preserving the consistency
	// guarantees of the API. This will become simpler once we have a notion of
	// unique tags.
	q := datastore.NewQuery("InstanceTag").
		Ancestor(PackageKey(c, pkg)).
		Eq("tag", common.JoinInstanceTag(tag)).
		KeysOnly(true).
		Limit(2)

	var tags []*Tag
	switch err := datastore.GetAll(c, q, &tags); {
	case err != nil:
		return "", errors.Annotate(err, "failed to query tags").Tag(transient.Tag).Err()
	case len(tags) == 0:
		return "", errors.Reason("no such tag").Tag(grpcutil.NotFoundTag).Err()
	case len(tags) > 1:
		return "", errors.Reason("ambiguity when resolving the tag, more than one instance has it").Tag(grpcutil.FailedPreconditionTag).Err()
	default:
		return tags[0].Instance.StringID(), nil
	}
}

// checkExistingTags takes a list of tags and returns the ones that exist
// in the datastore and ones that don't.
//
// Checks 'Tag' field on existing tags, thus safeguarding against malicious
// SHA1 collisions in TagID().
//
// Tags in 'miss' list have only their key and 'Tag' fields set and nothing
// more.
func checkExistingTags(c context.Context, inst *Instance, tags []*api.Tag) (exist, miss []*Tag, err error) {
	instKey := datastore.KeyForObj(c, inst)
	tagEnts := make([]*Tag, len(tags))
	for i, tag := range tags {
		tagEnts[i] = &Tag{
			ID:       TagID(tag),
			Instance: instKey,
		}
	}

	// Try to grab all entities and bail on unexpected errors.
	existCount := 0
	missCount := 0
	if err := datastore.Get(c, tagEnts); err != nil {
		merr, ok := err.(errors.MultiError)
		if !ok {
			return nil, nil, errors.Annotate(err, "failed to fetch tags").Tag(transient.Tag).Err()
		}
		for i, err := range merr {
			switch err {
			case nil:
				existCount++
			case datastore.ErrNoSuchEntity:
				missCount++
			default:
				return nil, nil, errors.Annotate(err, "failed to fetch tag %q", tags[i]).Tag(transient.Tag).Err()
			}
		}
	} else {
		existCount = len(tags)
	}

	if existCount != 0 {
		exist = make([]*Tag, 0, existCount)
	}
	if missCount != 0 {
		miss = make([]*Tag, 0, missCount)
	}

	for i, ent := range tagEnts {
		switch kv := common.JoinInstanceTag(tags[i]); {
		case ent.Tag == kv:
			exist = append(exist, ent)
		case ent.Tag == "":
			ent.Tag = kv // so the caller knows what tag this is
			miss = append(miss, ent)
		default: // ent.Tag != kv
			return nil, nil, errors.Reason("tag %q collides with tag %q, refusing to touch it", kv, ent.Tag).
				Tag(grpcutil.InternalTag).Err()
		}
	}

	return
}
