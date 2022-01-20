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

// Package uniquetestvariants provides APIs to store/query unique variants for
// each test to/from spanner.
package uniquetestvariants

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

// UniqueTestVariantID is a struct that identifies a UniqueTestVariant.
type UniqueTestVariantID struct {
	Realm       string
	TestID      string
	VariantHash string
}

// Key returns a UniqueTestVariant spanner key.
func (id UniqueTestVariantID) Key() spanner.Key {
	return spanner.Key{id.Realm, id.TestID, id.VariantHash}
}

// UniqueTestVariant is a struct that represents a unique variant.
// Each unique variant is identified by its Realm, TestID, and VariantHash.
type UniqueTestVariant struct {
	UniqueTestVariantID
	Variant        *pb.Variant
	LastRecordTime time.Time
}

// FromTestResults returns the unique test variants that occurred in the given
// testResults in a map, where the key is the `UniqueTestVariantID` and
// the value is the `UniqueTestVariant`.
func FromTestResults(realm string, testResults []*pb.TestResult) map[UniqueTestVariantID]*UniqueTestVariant {
	// Find all unique test variants.
	var ret = make(map[UniqueTestVariantID]*UniqueTestVariant)
	for _, r := range testResults {
		id := UniqueTestVariantID{
			Realm:       realm,
			TestID:      r.TestId,
			VariantHash: r.VariantHash,
		}
		ret[id] = &UniqueTestVariant{
			UniqueTestVariantID: id,
			Variant:             r.Variant,
			LastRecordTime:      time.Time{},
		}
	}

	return ret
}

// IDsFromMap returns the keys in a `map[UniqueTestVariantID]*UniqueTestVariant`.
func IDsFromMap(utvMap map[UniqueTestVariantID]*UniqueTestVariant) []UniqueTestVariantID {
	ret := make([]UniqueTestVariantID, 0, len(utvMap))
	for id := range utvMap {
		ret = append(ret, id)
	}
	return ret
}

// FromMap returns the values in a `map[UniqueTestVariantID]*UniqueTestVariant`.
func FromMap(utvMap map[UniqueTestVariantID]*UniqueTestVariant) []*UniqueTestVariant {
	ret := make([]*UniqueTestVariant, 0, len(utvMap))
	for _, utv := range utvMap {
		ret = append(ret, utv)
	}
	return ret
}

// FilterIDsRecordedAfter returns a list of unique test variant IDs in the
// given list with LastRecordTime greater than beforeTime.
func FilterIDsRecordedAfter(ctx context.Context, utvIDs []UniqueTestVariantID, beforeTime time.Time) ([]UniqueTestVariantID, error) {
	st := spanner.NewStatement(`
		SELECT Realm, TestId, VariantHash
		FROM UniqueTestVariants
		WHERE
			LastRecordTime > @beforeTime AND
			(Realm, TestId, VariantHash) in (
				SELECT (Realm, TestID, VariantHash)
				FROM UNNEST(@utvIds)
			)
	`)
	st.Params = map[string]interface{}{
		"utvIds":     utvIDs,
		"beforeTime": beforeTime,
	}

	ret := make([]UniqueTestVariantID, 0)

	var b spanutil.Buffer
	err := span.Query(ctx, st).Do(func(r *spanner.Row) error {
		utvID := UniqueTestVariantID{}
		if err := b.FromSpanner(r, &utvID.Realm, &utvID.TestID, &utvID.VariantHash); err != nil {
			return err
		}

		ret = append(ret, utvID)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return ret, nil
}

// InsertOrUpdate inserts the provided unique test variants to the spanner or
// update them if they already exist. Always set LastRecordTime to
// spanner.CommitTimestamp.
func InsertOrUpdate(ctx context.Context, utvs ...*UniqueTestVariant) {
	mutations := make([]*spanner.Mutation, 0, len(utvs))
	for _, utv := range utvs {
		row := map[string]interface{}{
			"Realm":          utv.Realm,
			"TestId":         utv.TestID,
			"VariantHash":    utv.VariantHash,
			"Variant":        utv.Variant,
			"LastRecordTime": spanner.CommitTimestamp,
		}
		mut := spanner.InsertOrUpdateMap("UniqueTestVariants", spanutil.ToSpannerMap(row))
		mutations = append(mutations, mut)
	}

	span.BufferWrite(ctx, mutations...)
}

// Read reterives the UniqueTestVariant for the given id.
func Read(ctx context.Context, id UniqueTestVariantID) (*UniqueTestVariant, error) {
	ret := &UniqueTestVariant{
		UniqueTestVariantID: id,
	}
	err := spanutil.ReadRow(ctx, "UniqueTestVariants", id.Key(), map[string]interface{}{
		"Variant":        &ret.Variant,
		"LastRecordTime": &ret.LastRecordTime,
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Query reterives the all the unique variants associated with the specified
// testId, in the specified realm.
func Query(ctx context.Context, req *pb.QueryUniqueTestVariantsRequest) (*pb.QueryUniqueTestVariantsResponse, error) {
	pageSize := int(req.PageSize)
	st := spanner.NewStatement(`
		SELECT Variant, VariantHash
		FROM UniqueTestVariants
		WHERE Realm = @realm AND TestId = @testId AND VariantHash >= @pageToken
		ORDER BY VariantHash ASC
		LIMIT @pageSize
	`)
	st.Params = map[string]interface{}{
		"realm":     req.Realm,
		"testId":    req.TestId,
		"pageToken": req.PageToken,
		// Query one more row so we know whether we have reached the last page.
		"pageSize": pageSize + 1,
	}

	utvs := make([]*pb.UniqueTestVariant, 0, req.PageSize)
	nextPageToken := ""

	var b spanutil.Buffer
	err := span.Query(ctx, st).Do(func(r *spanner.Row) error {
		utv := &pb.UniqueTestVariant{
			Realm:  req.Realm,
			TestId: req.TestId,
		}
		if err := b.FromSpanner(r, &utv.Variant, &utv.VariantHash); err != nil {
			return err
		}

		// If we got enough unique test variants, use the last one as the next page
		// token.
		if len(utvs) == pageSize {
			nextPageToken = utv.VariantHash
			return nil
		}

		utvs = append(utvs, utv)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &pb.QueryUniqueTestVariantsResponse{
		Variants:      utvs,
		NextPageToken: nextPageToken,
	}, nil
}
