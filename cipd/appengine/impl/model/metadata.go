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
	"bytes"
	"context"
	"sort"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/common"
)

// InstanceMetadata represents one instance metadata entry.
//
// It is a key-value pair (along with some additional attributes).
//
// The parent entity is the instance entity. ID is derived from
// the key-value pair, see common.InstanceMetadataFingerprint.
type InstanceMetadata struct {
	_kind  string                `gae:"$kind,InstanceMetadata"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	Fingerprint string         `gae:"$id"`     // see common.InstanceMetadataFingerprint
	Instance    *datastore.Key `gae:"$parent"` // a key of the corresponding Instance entity

	Key         string `gae:"key"`                  // the metadata key
	Value       []byte `gae:"value,noindex"`        // the metadata payload, can be big
	ContentType string `gae:"content_type,noindex"` // a content type (perhaps guessed)

	AttachedBy string    `gae:"attached_by"` // who added this metadata
	AttachedTs time.Time `gae:"attached_ts"` // when it was added
}

// Proto returns cipd.InstanceMetadata proto with information from this entity.
//
// Assumes the entity is valid.
func (md *InstanceMetadata) Proto() *api.InstanceMetadata {
	return &api.InstanceMetadata{
		Key:         md.Key,
		Value:       md.Value,
		ContentType: md.ContentType,
		Fingerprint: md.Fingerprint,
		AttachedBy:  md.AttachedBy,
		AttachedTs:  timestamppb.New(md.AttachedTs),
	}
}

// AttachMetadata transactionally attaches metadata to an instance.
//
// Mutates `md` in place by calculating fingerprints and "guessing" content
// type if necessary.
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
//	Internal on fingerprint collision.
func AttachMetadata(ctx context.Context, inst *Instance, md []*api.InstanceMetadata) error {
	now := clock.Now(ctx).UTC()
	who := string(auth.CurrentIdentity(ctx))

	// Calculate fingerprints and guess content type before the transaction, it is
	// relatively slow. Throw away duplicate entries.
	seen := stringset.New(len(md))
	filtered := md[:0]
	for _, m := range md {
		m.Fingerprint = common.InstanceMetadataFingerprint(m.Key, m.Value)
		if seen.Add(m.Fingerprint) {
			if m.ContentType == "" {
				if guessPlainText(m.Value) {
					m.ContentType = "text/plain"
				} else {
					m.ContentType = "application/octet-stream"
				}
			}
			filtered = append(filtered, m)
		}
	}
	md = filtered

	return Txn(ctx, "AttachMetadata", func(ctx context.Context) error {
		if err := CheckInstanceReady(ctx, inst); err != nil {
			return err
		}

		// Prepare to fetch everything from the datastore.
		instKey := datastore.KeyForObj(ctx, inst)
		ents := make([]*InstanceMetadata, len(md))
		for i, m := range md {
			ents[i] = &InstanceMetadata{
				Fingerprint: m.Fingerprint,
				Instance:    instKey,
			}
		}

		// For all existing entries, double check their key-value pair matches
		// the one we try to attach. If not, we've got a hash collision in the
		// fingerprint. This should be super rare, but it doesn't hurt to check
		// since we fetched the entity already.
		checkExisting := func(ent *InstanceMetadata, msg *api.InstanceMetadata) error {
			if ent.Key != msg.Key {
				return grpcutil.InternalTag.Apply(errors.Fmt("fingerprint %q matches two metadata keys %q and %q, aborting", ent.Fingerprint, ent.Key, msg.Key))
			}
			if !bytes.Equal(ent.Value, msg.Value) {
				return grpcutil.InternalTag.Apply(errors.Fmt("fingerprint %q matches metadata key %q with two different values, aborting", ent.Fingerprint, ent.Key))
			}
			return nil
		}

		// Find entries that don't exist yet. We don't want to blindly overwrite
		// existing entries, since we want to preserve their AttachedBy/AttachedTs
		// etc. and skip emitting INSTANCE_METADATA_ATTACHED event log entries.
		missing := make([]*InstanceMetadata, 0, len(ents))
		if err := datastore.Get(ctx, ents); err != nil {
			merr, ok := err.(errors.MultiError)
			if !ok {
				return transient.Tag.Apply(errors.Fmt("failed to fetch metadata: %w", err))
			}
			for i, err := range merr {
				switch err {
				case nil:
					if err := checkExisting(ents[i], md[i]); err != nil {
						return err
					}
				case datastore.ErrNoSuchEntity:
					// Populate the rest of the entity fields from input proto fields.
					ent, msg := ents[i], md[i]
					ent.Key = msg.Key
					ent.Value = msg.Value
					ent.ContentType = msg.ContentType
					ent.AttachedBy = who
					ent.AttachedTs = now
					missing = append(missing, ent)
				default:
					return transient.Tag.Apply(errors.Fmt("failed to fetch metadata %q: %w", ents[i].Fingerprint, err))
				}
			}
		} else {
			// No error at all => all entries already exist, just check them.
			for i := range ents {
				if err := checkExisting(ents[i], md[i]); err != nil {
					return err
				}
			}
		}

		if len(missing) == 0 {
			return nil
		}

		// Store everything.
		if err := datastore.Put(ctx, missing); err != nil {
			return transient.Tag.Apply(err)
		}
		return flushToEventLog(ctx, missing, api.EventKind_INSTANCE_METADATA_ATTACHED, inst, who, now)
	})
}

// DetachMetadata detaches a bunch of metadata entries from an instance.
//
// Assumes inputs are already validated. If Fingerprint is populated, uses it
// to identifies entries to detach. Otherwise calculates it from Key and Value
// (which must be populated in this case).
//
// Launches a transaction inside (and thus can't be a part of a transaction
// itself).
func DetachMetadata(ctx context.Context, inst *Instance, md []*api.InstanceMetadata) error {
	now := clock.Now(ctx).UTC()
	who := string(auth.CurrentIdentity(ctx))

	// Calculate fingerprints before the transaction, it is relatively slow. Throw
	// away duplicate entries.
	seen := stringset.New(len(md))
	filtered := md[:0]
	for _, m := range md {
		if m.Fingerprint == "" {
			m.Fingerprint = common.InstanceMetadataFingerprint(m.Key, m.Value)
		}
		if seen.Add(m.Fingerprint) {
			filtered = append(filtered, m)
		}
	}
	md = filtered

	return Txn(ctx, "DetachMetadata", func(ctx context.Context) error {
		// Prepare to fetch everything from the datastore to figure out what entries
		// actually exist, for the event log.
		instKey := datastore.KeyForObj(ctx, inst)
		ents := make([]*InstanceMetadata, len(md))
		for i, m := range md {
			ents[i] = &InstanceMetadata{
				Fingerprint: m.Fingerprint,
				Instance:    instKey,
			}
		}

		existing := make([]*InstanceMetadata, 0, len(ents))
		if err := datastore.Get(ctx, ents); err != nil {
			merr, ok := err.(errors.MultiError)
			if !ok {
				return transient.Tag.Apply(errors.Fmt("failed to fetch metadata: %w", err))
			}
			for i, err := range merr {
				switch err {
				case nil:
					existing = append(existing, ents[i])
				case datastore.ErrNoSuchEntity:
					// Skip, that's ok.
				default:
					return transient.Tag.Apply(errors.Fmt("failed to fetch metadata %q: %w", ents[i].Fingerprint, err))
				}
			}
		} else {
			existing = ents
		}

		if len(existing) == 0 {
			return nil
		}

		// Store everything.
		if err := datastore.Delete(ctx, existing); err != nil {
			return transient.Tag.Apply(err)
		}
		return flushToEventLog(ctx, existing, api.EventKind_INSTANCE_METADATA_DETACHED, inst, who, now)
	})
}

// ListMetadata lists all instance metadata.
//
// The result is ordered by AttachedTs (the most recent first).
func ListMetadata(ctx context.Context, inst *Instance) ([]*InstanceMetadata, error) {
	// Note: 'Order' here is unnecessary, since we sort in memory later anyhow.
	// But it is here in an anticipation of eventually implementing pagination.
	q := datastore.NewQuery("InstanceMetadata").
		Ancestor(datastore.KeyForObj(ctx, inst)).
		Order("-attached_ts")

	var out []*InstanceMetadata
	if err := datastore.GetAll(ctx, q, &out); err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("datastore query failed: %w", err))
	}
	orderByTsAndKey(out)

	return out, nil
}

// ListMetadataWithKeys lists instance metadata with any of the given keys.
//
// The result is ordered by AttachedTs (the most recent first).
func ListMetadataWithKeys(ctx context.Context, inst *Instance, keys []string) ([]*InstanceMetadata, error) {
	if len(keys) == 0 {
		panic("must not be empty")
	}

	qs := make([]*datastore.Query, len(keys))
	for i, key := range keys {
		qs[i] = datastore.NewQuery("InstanceMetadata").
			Ancestor(datastore.KeyForObj(ctx, inst)).
			Eq("key", key).
			Order("-attached_ts")
	}

	var out []*InstanceMetadata
	err := datastore.RunMulti(ctx, qs, func(md *InstanceMetadata) {
		out = append(out, md)
	})
	if err != nil {
		return nil, transient.Tag.Apply(errors.Fmt("datastore query failed: %w", err))
	}
	orderByTsAndKey(out)

	return out, nil
}

// orderByTsAndKey order entries by (-AttachedTs, Key).
func orderByTsAndKey(md []*InstanceMetadata) {
	sort.Slice(md, func(i, j int) bool {
		l, r := md[i], md[j]
		if l.AttachedTs.Equal(r.AttachedTs) {
			return l.Key < r.Key
		}
		return l.AttachedTs.After(r.AttachedTs)
	})
}

// flushToEventLog emits a bunch of event log entries with metadata.
func flushToEventLog(ctx context.Context, ents []*InstanceMetadata, kind api.EventKind, inst *Instance, who string, now time.Time) error {
	nowTS := timestamppb.New(now)
	events := Events{}
	for _, ent := range ents {
		// Export only valid UTF-8 values of known text-like content types.
		mdValue := ""
		if IsTextContentType(ent.ContentType) {
			mdValue = string(ent.Value)
			if !utf8.ValidString(mdValue) {
				mdValue = ""
			}
		}
		events.Emit(&api.Event{
			Kind:          kind,
			Package:       inst.Package.StringID(),
			Instance:      inst.InstanceID,
			Who:           who,
			When:          nowTS,
			MdKey:         ent.Key,
			MdValue:       mdValue,
			MdContentType: ent.ContentType,
			MdFingerprint: ent.Fingerprint,
		})
	}
	return events.Flush(ctx)
}

// guessPlainText returns true for smallish printable ASCII strings.
func guessPlainText(v []byte) bool {
	if len(v) >= 32768 {
		return false
	}
	for _, b := range v {
		// Acceptable non-printable chars.
		if b == '\r' || b == '\n' || b == '\t' {
			continue
		}
		// Everything else should be from a printable ASCII range.
		if b < ' ' || b >= 0x7F {
			return false
		}
	}
	return true
}
