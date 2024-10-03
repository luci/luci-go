// Copyright 2023 The LUCI Authors.
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

// Package gerritchangelists contains methods for maintaining a cache
// of whether gerrit changelists were authored by humans or automation.
package gerritchangelists

import (
	"context"
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// GerritChangelist is a record used to cache whether a gerrit changelist
// was authored by a human or by automation.
//
// The cache is per-project to avoid confused deputy problems. Each project
// will use its own project-scoped service account to access to gerrit to
// retrieve change owner information and store this data in its own cache.
type GerritChangelist struct {
	// Project is the name of the LUCI Project. This is the project
	// for which the cache is being maintained and the project we authenticated
	// as to gerrit.
	Project string
	// Host is the gerrit hostname. E.g. "chromium-review.googlesource.com".
	Host string
	// Change is the gerrit change number.
	Change int64
	// The kind of owner that created the changelist. This could be
	// a human (user) or automation.
	OwnerKind pb.ChangelistOwnerKind
	// The time the record was created in Spanner.
	CreationTime time.Time
}

// Key represents the key fields of a GerritChangelist record.
type Key struct {
	// Project is the name of the LUCI Project.
	Project string
	// Host is the gerrit hostname. E.g. "chromium-review.googlesource.com".
	Host string
	// Change is the gerrit change number.
	Change int64
}

// Read reads the gerrit changelists with the given keys.
//
// ctx must be a Spanner transaction context.
func Read(ctx context.Context, keys map[Key]struct{}) (map[Key]*GerritChangelist, error) {
	var ks []spanner.Key
	for key := range keys {
		ks = append(ks, spanner.Key{key.Project, key.Host, key.Change})
	}
	return readByKeys(ctx, spanner.KeySetFromKeys(ks...))
}

// ReadAll reads all gerrit changelists. Provided for testing only.
func ReadAll(ctx context.Context) ([]*GerritChangelist, error) {
	entries, err := readByKeys(ctx, spanner.AllKeys())
	if err != nil {
		return nil, err
	}
	var results []*GerritChangelist
	for _, cl := range entries {
		results = append(results, cl)
	}
	// Sort changelists by key.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Project != results[j].Project {
			return results[i].Project < results[j].Project
		}
		if results[i].Host != results[j].Host {
			return results[i].Host < results[j].Host
		}
		return results[i].Change < results[j].Change
	})
	return results, nil
}

// readByKeys reads gerrit changelists for the given keyset.
func readByKeys(ctx context.Context, keys spanner.KeySet) (map[Key]*GerritChangelist, error) {
	results := make(map[Key]*GerritChangelist)
	var b spanutil.Buffer

	it := span.Read(ctx, "GerritChangelists", keys, []string{"Project", "Host", "Change", "OwnerKind", "CreationTime"})
	err := it.Do(func(r *spanner.Row) error {
		var changelist GerritChangelist

		err := b.FromSpanner(r,
			&changelist.Project,
			&changelist.Host,
			&changelist.Change,
			&changelist.OwnerKind,
			&changelist.CreationTime,
		)
		if err != nil {
			return errors.Annotate(err, "read row").Err()
		}
		key := Key{
			Project: changelist.Project,
			Host:    changelist.Host,
			Change:  changelist.Change,
		}
		results[key] = &changelist
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "read gerrit changelists").Err()
	}
	return results, nil
}

// CreateOrUpdate returns a Spanner mutation that inserts the given
// gerrit changelist record.
func CreateOrUpdate(g *GerritChangelist) (*spanner.Mutation, error) {
	if err := validateGerritChangelist(g); err != nil {
		return nil, err
	}

	// Row not found. Create it.
	row := map[string]any{
		"Project":      g.Project,
		"Host":         g.Host,
		"Change":       g.Change,
		"OwnerKind":    g.OwnerKind,
		"CreationTime": spanner.CommitTimestamp,
	}
	return spanner.InsertOrUpdateMap("GerritChangelists", spanutil.ToSpannerMap(row)), nil
}

// validateGerritChangelist validates that the GerritChangelist is valid.
func validateGerritChangelist(g *GerritChangelist) error {
	if err := pbutil.ValidateProject(g.Project); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if g.Host == "" || len(g.Host) > 255 {
		return errors.Reason("host: must have a length between 1 and 255").Err()
	}
	if g.Change <= 0 {
		return errors.Reason("change: must be set and positive").Err()
	}
	return nil
}

// SetGerritChangelistsForTesting replaces the set of stored gerrit changelists
// to match the given set. Provided for testing only.
func SetGerritChangelistsForTesting(ctx context.Context, t testing.TB, gs []*GerritChangelist) error {
	t.Helper()
	testutil.MustApply(ctx, t,
		spanner.Delete("GerritChangelists", spanner.AllKeys()))
	// Insert some GerritChangelists.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, g := range gs {
			row := map[string]any{
				"Project":      g.Project,
				"Host":         g.Host,
				"Change":       g.Change,
				"OwnerKind":    g.OwnerKind,
				"CreationTime": g.CreationTime,
			}
			span.BufferWrite(ctx, spanner.InsertMap("GerritChangelists", spanutil.ToSpannerMap(row)))
		}
		return nil
	})
	return err
}
