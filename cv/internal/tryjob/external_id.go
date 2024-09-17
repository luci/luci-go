// Copyright 2021 The LUCI Authors.
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

package tryjob

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
)

// ExternalID is a unique ID deterministically constructed to identify Tryjobs.
//
// Currently, only Buildbucket is supported.
type ExternalID string

// BuildbucketID makes an ExternalID for a Buildbucket build.
//
// Host is typically "cr-buildbucket.appspot.com".
// Build is a number, e.g. 8839722009404151168 for
// https://ci.chromium.org/ui/p/infra/builders/try/infra-try-bionic-64/b8839722009404151168/overview
func BuildbucketID(host string, build int64) (ExternalID, error) {
	if strings.ContainsRune(host, '/') {
		return "", errors.Reason("invalid host %q: must not contain /", host).Err()
	}
	return ExternalID(fmt.Sprintf("buildbucket/%s/%d", host, build)), nil
}

// MustBuildbucketID is like `BuildbucketID()` but panics on error.
func MustBuildbucketID(host string, build int64) ExternalID {
	ret, err := BuildbucketID(host, build)
	if err != nil {
		panic(err)
	}
	return ret
}

// ParseBuildbucketID returns the Buildbucket host and build if this is a
// BuildbucketID.
func (e ExternalID) ParseBuildbucketID() (host string, build int64, err error) {
	parts := strings.Split(string(e), "/")
	if len(parts) != 3 || parts[0] != "buildbucket" {
		err = errors.Reason("%q is not a valid BuildbucketID", e).Err()
		return
	}
	host = parts[1]
	build, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		err = errors.Annotate(err, "%q is not a valid BuildbucketID", e).Err()
	}
	return
}

// MustParseBuildbucketID is like `ParseBuildbucketID` but panics on error
func (e ExternalID) MustParseBuildbucketID() (string, int64) {
	host, build, err := e.ParseBuildbucketID()
	if err != nil {
		panic(err)
	}
	return host, build
}

// URL returns the Buildbucket URL of the Tryjob.
func (e ExternalID) URL() (string, error) {
	switch kind, err := e.Kind(); {
	case err != nil:
		return "", err
	case kind == "buildbucket":
		host, build, err := e.ParseBuildbucketID()
		if err != nil {
			return "", errors.Annotate(err, "invalid tryjob.ExternalID").Err()
		}
		return fmt.Sprintf("https://%s/build/%d", host, build), nil
	default:
		return "", errors.Reason("unrecognized ExternalID: %q", e).Err()
	}
}

// MustURL is like `URL()` but panics on err.
func (e ExternalID) MustURL() string {
	ret, err := e.URL()
	if err != nil {
		panic(err)
	}
	return ret
}

// Kind identifies the backend that corresponds to the tryjob this ExternalID
// applies to.
func (e ExternalID) Kind() (string, error) {
	s := string(e)
	idx := strings.IndexRune(s, '/')
	if idx <= 0 {
		return "", errors.Reason("invalid ExternalID: %q", s).Err()
	}
	return s[:idx], nil
}

// Load looks up a Tryjob entity.
//
// If an entity referred to by the ExternalID does not exist in CV,
// `nil, nil` will be returned.
func (e ExternalID) Load(ctx context.Context) (*Tryjob, error) {
	tjm := tryjobMap{ExternalID: e}
	switch err := datastore.Get(ctx, &tjm); err {
	case nil:
		break
	case datastore.ErrNoSuchEntity:
		return nil, nil
	default:
		return nil, errors.Annotate(err, "resolving ExternalID %q to a Tryjob", e).Tag(transient.Tag).Err()
	}

	res := &Tryjob{ID: tjm.InternalID}
	if err := datastore.Get(ctx, res); err != nil {
		// It is unlikely that we'll find a tryjobMap referencing a Tryjob that
		// doesn't exist. And if we do it'll most likely be due to a retention
		// policy removing old entities, so the tryjobMap entity will be
		// removed soon as well.
		return nil, errors.Annotate(err, "retrieving Tryjob with ExternalID %q", e).Tag(transient.Tag).Err()
	}
	return res, nil
}

// MustLoad is like `Load` but panics on error.
func (e ExternalID) MustLoad(ctx context.Context) *Tryjob {
	tj, err := e.Load(ctx)
	if err != nil {
		panic(err)
	}
	return tj
}

// MustCreateIfNotExists is intended for testing only.
//
// If a Tryjob with this ExternalID exists, the Tryjob is loaded from
// datastore. If it does not, it is created, saved and returned.
//
// Panics on error.
func (e ExternalID) MustCreateIfNotExists(ctx context.Context) *Tryjob {
	// Quick read-only path.
	if tryjob, err := e.Load(ctx); err == nil && tryjob != nil {
		return tryjob
	}
	// Transaction path.
	var tryjob *Tryjob
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		tryjob, err = e.Load(ctx)
		switch {
		case err != nil:
			return err
		case tryjob != nil:
			return nil
		}
		now := datastore.RoundTime(clock.Now(ctx).UTC())
		tryjob = &Tryjob{
			ExternalID:       e,
			EVersion:         1,
			EntityCreateTime: now,
			EntityUpdateTime: now,
		}
		if err := datastore.AllocateIDs(ctx, tryjob); err != nil {
			return err
		}
		m := tryjobMap{ExternalID: e, InternalID: tryjob.ID}
		return datastore.Put(ctx, &m, tryjob)
	}, nil)
	if err != nil {
		panic(err)
	}
	return tryjob
}

// Resolve resolves the ExternalID to internal TryjobID.
//
// Returns zero TryjobID if the ExternalID is not mapped to any Tryjob.
func (e ExternalID) Resolve(ctx context.Context) (common.TryjobID, error) {
	tjm := &tryjobMap{ExternalID: e}
	switch err := datastore.Get(ctx, tjm); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return 0, nil
	case err != nil:
		return 0, errors.Annotate(err, "failed to load tryjobMap").Tag(transient.Tag).Err()
	default:
		return tjm.InternalID, nil
	}
}

// Resolve converts ExternalIDs to internal TryjobIDs.
func Resolve(ctx context.Context, eids ...ExternalID) (common.TryjobIDs, error) {
	tjms := make([]tryjobMap, len(eids))
	for i, eid := range eids {
		tjms[i].ExternalID = eid
	}

	if errs := datastore.Get(ctx, tjms); errs != nil {
		merr, _ := errs.(errors.MultiError)
		if merr == nil {
			return nil, errors.Annotate(errs, "failed to load tryjobMaps").Tag(transient.Tag).Err()
		}
		for _, err := range merr {
			if err != nil && err != datastore.ErrNoSuchEntity {
				return nil, errors.Annotate(common.MostSevereError(merr), "resolving ExternalIDs").Tag(transient.Tag).Err()
			}
		}
	}

	ret := make(common.TryjobIDs, len(eids))
	for i, tjm := range tjms {
		ret[i] = tjm.InternalID
	}
	return ret, nil
}

// MustResolve is like `Resolve` but panics on error
func MustResolve(ctx context.Context, eids ...ExternalID) common.TryjobIDs {
	tryjobIDs, err := Resolve(ctx, eids...)
	if err != nil {
		panic(err)
	}
	return tryjobIDs
}

// ResolveToTryjobs resolves ExternalIDs to Tryjob entities.
//
// If the external id can't be found inside CV, its corresponding Tryjob
// entity will be nil.
func ResolveToTryjobs(ctx context.Context, eids ...ExternalID) ([]*Tryjob, error) {
	tjids, err := Resolve(ctx, eids...)
	if err != nil {
		return nil, err
	}
	ret := make([]*Tryjob, len(tjids))
	var toLoad []*Tryjob
	for i, id := range tjids {
		if id != 0 {
			ret[i] = &Tryjob{ID: id}
			toLoad = append(toLoad, ret[i])
		}
	}
	if len(toLoad) > 0 {
		if err := datastore.Get(ctx, toLoad); err != nil {
			return nil, errors.Annotate(err, "failed to load tryjobs").Tag(transient.Tag).Err()
		}
	}
	return ret, nil
}
