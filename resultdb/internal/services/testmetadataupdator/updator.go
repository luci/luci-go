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

// Package testmetadataupdator implements a task which ingest test metadata from invocations.
package testmetadataupdator

import (
	"context"
	"math"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testmetadata"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Expiration time of a test metadata.
// After expiry, update to less metadata information is allowed.
var metadataExpiry = 48 * time.Hour

// Maximum number of incoming test metadata we hold in memory.
var batchSize = 1000

type testID string

// testMetadataUpdator updates TestMetadata rows with test results in a set of invocations.
type testMetadataUpdator struct {
	sources *pb.Sources
	invIDs  invocations.IDSet
	start   time.Time // Used to determine if a test metadata is expired.
}

func newUpdator(sources *pb.Sources, invIDs invocations.IDSet, start time.Time) *testMetadataUpdator {
	return &testMetadataUpdator{sources: sources, invIDs: invIDs, start: start}
}

func (u *testMetadataUpdator) run(ctx context.Context) error {
	for invID := range u.invIDs {
		// Query test results and use them to update test metadata.
		batchC := make(chan map[testID]*pb.TestMetadata)
		// Batch update test metadata.
		eg, ctx := errgroup.WithContext(ctx)

		eg.Go(func() error {
			defer close(batchC)
			if err := u.queryTestResultMetadata(ctx, invID, batchC); err != nil {
				return errors.Fmt("query test result metadata, invocation %s: %w", invID, err)
			}
			return nil
		})

		eg.Go(func() error {
			var realm string
			if err := invocations.ReadColumns(span.Single(ctx), invID, map[string]any{"Realm": &realm}); err != nil {
				return errors.Fmt("read realm, invocation %s: %w", invID, err)
			}
			if err := u.batchCreateOrUpdateTestMetadata(ctx, realm, batchC); err != nil {
				return errors.Fmt("create or update test metadata, invocation %s: %w", invID, err)
			}
			return nil
		})
		if err := eg.Wait(); err != nil {
			return err
		}
	}
	return nil
}

// queryTestResultMetadata visits all test results in the given invocations.
// and returns one test metadata with the most metadata information for each test.
func (u *testMetadataUpdator) queryTestResultMetadata(ctx context.Context, invID invocations.ID, batchC chan<- map[testID]*pb.TestMetadata) error {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	q := testresults.Query{
		InvocationIDs: invocations.NewIDSet(invID),
		Mask: mask.MustFromReadMask(&pb.TestResult{},
			"test_id",
			"test_metadata",
		),
	}
	deduplicatedMetadata := make(map[testID]*pb.TestMetadata, batchSize)
	err := q.Run(ctx, func(tr *pb.TestResult) error {
		existing, ok := deduplicatedMetadata[testID(tr.TestId)]
		if !ok {
			if len(deduplicatedMetadata) >= batchSize {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case batchC <- deduplicatedMetadata:
				}
				deduplicatedMetadata = make(map[testID]*pb.TestMetadata, batchSize)
			}
			deduplicatedMetadata[testID(tr.TestId)] = tr.TestMetadata
			return nil
		}
		if fieldExistenceBitField(existing) < fieldExistenceBitField(tr.TestMetadata) {
			deduplicatedMetadata[testID(tr.TestId)] = tr.TestMetadata
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(deduplicatedMetadata) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case batchC <- deduplicatedMetadata:
		}
	}
	return nil
}

func (u *testMetadataUpdator) batchCreateOrUpdateTestMetadata(ctx context.Context, realm string, batchC <-chan map[testID]*pb.TestMetadata) error {
	for testResults := range batchC {
		if err := u.updateOrCreateRows(ctx, realm, testResults); err != nil {
			return err
		}
	}
	return nil
}

func (u *testMetadataUpdator) updateOrCreateRows(ctx context.Context, realm string, testMetadata map[testID]*pb.TestMetadata) error {
	project, subRealm := realms.Split(realm)
	testIDs := make([]string, 0, len(testMetadata))
	for k := range testMetadata {
		testIDs = append(testIDs, string(k))
	}
	f := func(ctx context.Context) error {
		seen := make(map[string]bool)
		ms := []*spanner.Mutation{}
		opts := testmetadata.ReadTestMetadataOptions{
			Project:   project,
			TestIDs:   testIDs,
			SourceRef: pbutil.SourceRefFromSources(u.sources),
			SubRealm:  subRealm,
		}
		// Update existing metadata entries.
		err := testmetadata.ReadTestMetadata(ctx, opts, func(tmd *testmetadata.TestMetadataRow) error {
			seen[tmd.TestID] = true
			// No update if test result from a lower commit position than existing test metadata.
			if u.sources.GitilesCommit.Position < tmd.Position {
				return nil
			}
			newMetadata := testMetadata[testID(tmd.TestID)]
			newBitField := fieldExistenceBitField(newMetadata)
			existingBitField := fieldExistenceBitField(tmd.TestMetadata)
			// If test result from the same commit position,
			// then only update when more metadata fields were found in the test results.
			// If test result from a higher commit position,
			// then update when more or equal number of metadata fields were found in the test results
			// OR existing test metadata expired.
			if (u.sources.GitilesCommit.Position == tmd.Position && newBitField > existingBitField) ||
				(u.sources.GitilesCommit.Position > tmd.Position &&
					(tmd.LastUpdated.Add(metadataExpiry).Before(u.start) || newBitField >= existingBitField)) {
				mutation := u.testMetadataMutation(project, tmd.TestID, subRealm, testMetadata[testID(tmd.TestID)])
				ms = append(ms, mutation)
			}
			return nil
		})
		if err != nil {
			return err
		}
		// Create new metadata entries which aren't exist yet.
		for testID, tm := range testMetadata {
			if !seen[string(testID)] {
				mutation := u.testMetadataMutation(project, string(testID), subRealm, tm)
				ms = append(ms, mutation)
			}
		}
		span.BufferWrite(ctx, ms...)
		return nil
	}
	_, err := span.ReadWriteTransaction(ctx, f)
	if err != nil {
		return err
	}
	return nil
}

var saveCols = []string{
	"Project",
	"TestId",
	"RefHash",
	"SubRealm",
	"LastUpdated",
	"TestMetadata",
	"SourceRef",
	"Position",
}

func (u *testMetadataUpdator) testMetadataMutation(project, testID, subRealm string, tm *pb.TestMetadata) *spanner.Mutation {
	ref := pbutil.SourceRefFromSources(u.sources)
	vals := []any{
		project,
		testID,
		pbutil.SourceRefHash(ref),
		subRealm,
		spanner.CommitTimestamp,
		spanutil.Compressed(pbutil.MustMarshal(tm)).ToSpanner(),
		spanutil.Compressed(pbutil.MustMarshal(ref)).ToSpanner(),
		u.sources.GitilesCommit.Position,
	}
	return spanner.InsertOrUpdate("TestMetadata", saveCols, vals)
}

// fieldExistenceBitField returns bit fields contains 0 or 1 in each bit to indicate
// the existence of each field in the test metadata.
// Fields from the lowest bit to highest bit:
// TestMetadata.name
// TestMetadata.location.repo
// TestMetadata.location.file_name
// TestMetadata.location.line
// TestMetadata.bug_component
// TestMetadata.properties
func fieldExistenceBitField(metadata *pb.TestMetadata) uint8 {
	bitField := uint8(0)
	bitFieldOrder := []string{
		"name",
		"location.repo",
		"location.file_name",
		"location.line",
		"bug_component",
		"properties",
	}
	for i, k := range bitFieldOrder {
		if exist(strings.Split(k, "."), metadata.ProtoReflect()) {
			bitField += uint8(math.Pow(2, float64(i)))
		}
	}
	return bitField
}

func exist(fieldNameTokens []string, message protoreflect.Message) bool {
	if len(fieldNameTokens) == 0 {
		return true
	}
	curName := fieldNameTokens[0]
	fd := message.Descriptor().Fields().ByTextName(curName)
	if message.Has(fd) {
		if fd.Kind() == protoreflect.MessageKind {
			return exist(fieldNameTokens[1:], message.Get(fd).Message())
		}
		return len(fieldNameTokens) == 1
	}
	return false
}
