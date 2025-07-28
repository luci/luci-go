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

package changelist

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
)

// PanicIfNotValid checks that Snapshot stored has required fields set.
func (s *Snapshot) PanicIfNotValid() {
	switch {
	case s == nil:
	case s.GetExternalUpdateTime() == nil:
		panic("missing ExternalUpdateTime")
	case s.GetLuciProject() == "":
		panic("missing LuciProject")
	case s.GetMinEquivalentPatchset() == 0:
		panic("missing MinEquivalentPatchset")
	case s.GetPatchset() == 0:
		panic("missing Patchset")

	case s.GetGerrit() == nil:
		panic("Gerrit is required, until CV supports more code reviews")
	case s.GetGerrit().GetInfo() == nil:
		panic("Gerrit.Info is required, until CV supports more code reviews")
	}
}

// LoadCLsMap loads `CL` entities which are values in the provided map.
//
// Updates `CL` entities *in place*, but also returns them as a slice.
func LoadCLsMap(ctx context.Context, m map[common.CLID]*CL) ([]*CL, error) {
	cls := make([]*CL, 0, len(m))
	for _, cl := range m {
		cls = append(cls, cl)
	}
	return loadCLs(ctx, cls)
}

// LoadCLsByIDs loads `CL` entities of the provided list of clids.
func LoadCLsByIDs(ctx context.Context, clids common.CLIDs) ([]*CL, error) {
	cls := make([]*CL, len(clids))
	for i, clid := range clids {
		cls[i] = &CL{ID: clid}
	}
	return loadCLs(ctx, cls)
}

// LoadCLs loads given `CL` entities.
func LoadCLs(ctx context.Context, cls []*CL) error {
	_, err := loadCLs(ctx, cls)
	return err
}

func loadCLs(ctx context.Context, cls []*CL) ([]*CL, error) {
	err := datastore.Get(ctx, cls)
	switch merr, ok := err.(errors.MultiError); {
	case err == nil:
		return cls, nil
	case ok:
		for i, err := range merr {
			if err == datastore.ErrNoSuchEntity {
				return nil, errors.Fmt("CL %d not found in Datastore", cls[i].ID)
			}
		}
		count, err := merr.Summary()
		return nil, transient.Tag.Apply(errors.Fmt("failed to load %d out of %d CLs: %w", count, len(cls), err))
	default:
		return nil, transient.Tag.Apply(errors.Fmt("failed to load %d CLs: %w", len(cls), err))
	}
}

// RemoveUnusedGerritInfo mutates given ChangeInfo to remove what CV definitely
// doesn't need to reduce bytes shuffled to/from Datastore.
//
// Doesn't complain if anything is missing.
//
// NOTE: keep this function actions in sync with storage.proto doc for
// Gerrit.info field.
func RemoveUnusedGerritInfo(ci *gerritpb.ChangeInfo) {
	const keepEmail = true
	const removeEmail = false
	cleanUser := func(u *gerritpb.AccountInfo, keepEmail bool) {
		if u == nil {
			return
		}
		u.SecondaryEmails = nil
		u.Name = ""
		u.Username = ""
		if !keepEmail {
			u.Email = ""
		}
	}

	cleanRevision := func(r *gerritpb.RevisionInfo) {
		if r == nil {
			return
		}
		cleanUser(r.GetUploader(), keepEmail)
		r.Description = "" // patchset title.
		if c := r.GetCommit(); c != nil {
			c.Message = ""
			c.Author = nil
		}
		r.Files = nil
	}

	cleanMessage := func(m *gerritpb.ChangeMessageInfo) {
		if m == nil {
			return
		}
		cleanUser(m.GetAuthor(), removeEmail)
		cleanUser(m.GetRealAuthor(), removeEmail)
	}

	cleanLabel := func(l *gerritpb.LabelInfo) {
		if l == nil {
			return
		}
		all := l.GetAll()[:0]
		for _, a := range l.GetAll() {
			if a.GetValue() == 0 {
				continue
			}
			cleanUser(a.GetUser(), keepEmail)
			all = append(all, a)
		}
		l.All = all
	}

	for _, r := range ci.GetRevisions() {
		cleanRevision(r)
	}
	for _, m := range ci.GetMessages() {
		cleanMessage(m)
	}
	for _, l := range ci.GetLabels() {
		cleanLabel(l)
	}
	cleanUser(ci.GetOwner(), keepEmail)
}

// OwnerIdentity is the identity of a user owning this CL.
//
// Snapshot must not be nil.
func (s *Snapshot) OwnerIdentity() (identity.Identity, error) {
	if s == nil {
		panic("Snapshot is nil")
	}

	g := s.GetGerrit()
	if g == nil {
		return "", errors.New("non-Gerrit CLs not supported")
	}
	owner := g.GetInfo().GetOwner()
	if owner == nil {
		panic("Snapshot Gerrit has no owner. Bug in gerrit/updater")
	}
	email := owner.GetEmail()
	if email == "" {
		return "", errors.Fmt("CL %s/%d owner email of account %d is unknown",
			g.GetHost(), g.GetInfo().GetNumber(),
			owner.GetAccountId())
	}
	return identity.MakeIdentity("user:" + email)
}

// IsSubmittable returns whether the change has been approved
// by the project submit rules.
func (s *Snapshot) IsSubmittable() (bool, error) {
	if s == nil {
		panic("Snapshot is nil")
	}

	g := s.GetGerrit()
	if g == nil {
		return false, errors.New("non-Gerrit CLs not supported")
	}
	return g.GetInfo().GetSubmittable(), nil
}

// IsSubmitted returns whether the change has been submitted.
func (s *Snapshot) IsSubmitted() (bool, error) {
	if s == nil {
		panic("Snapshot is nil")
	}

	g := s.GetGerrit()
	if g == nil {
		return false, errors.New("non-Gerrit CLs not supported")
	}
	return g.GetInfo().GetStatus() == gerritpb.ChangeStatus_MERGED, nil
}

// QueryCLIDsUpdatedBefore queries all CLIDs updated before the given timestamp.
//
// This is mainly used for data retention purpose. Result CLIDs are sorted.
func QueryCLIDsUpdatedBefore(ctx context.Context, before time.Time) (common.CLIDs, error) {
	var ret common.CLIDs
	var retMu sync.Mutex
	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(10)
	for shard := range retentionKeyShards {
		eg.Go(func() error {
			q := datastore.NewQuery("CL").
				Lt("RetentionKey", fmt.Sprintf("%02d/%010d", shard, before.Unix())).
				Gt("RetentionKey", fmt.Sprintf("%02d/", shard)).
				KeysOnly(true)
			var keys []*datastore.Key
			switch err := datastore.GetAll(ectx, q, &keys); {
			case err != nil:
				return transient.Tag.Apply(errors.Fmt("failed to query CL keys: %w", err))
			case len(keys) > 0:
				retMu.Lock()
				for _, key := range keys {
					ret = append(ret, common.CLID(key.IntID()))
				}
				retMu.Unlock()
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	sort.Sort(ret)
	return ret, nil
}
