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

// Package testmetadata implements methods to query from TestMetadata spanner table.
package testmetadata

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TestMetadataRow represents a row in the TestMetadata table.
type TestMetadataRow struct {
	Project      string
	TestID       string
	RefHash      []byte
	SubRealm     string
	LastUpdated  time.Time
	TestMetadata *pb.TestMetadata
	SourceRef    *pb.SourceRef
	Position     int64
}

// ReadTestMetadataOptions is the input for the ReadTestMetadata function.
type ReadTestMetadataOptions struct {
	Project   string
	SubRealm  string
	TestIDs   []string
	SourceRef *pb.SourceRef
}

// ReadTestMetadata read rows from TestMetadata table.
func ReadTestMetadata(ctx context.Context, opts ReadTestMetadataOptions, f func(*TestMetadataRow) error) error {
	st := spanner.NewStatement(`
	SELECT
	 tm.Project,
	 tm.TestId,
	 tm.RefHash,
	 tm.SubRealm,
	 tm.LastUpdated,
	 tm.SourceRef,
	 tm.TestMetadata,
	 tm.Position,
	FROM TestMetadata tm
	WHERE tm.Project = @project
		AND tm.TestId in UNNEST(@testIDs)
		AND tm.RefHash = @refHash
		 AND tm.SubRealm = @subRealm`)
	st.Params = spanutil.ToSpannerMap(map[string]any{
		"project":  opts.Project,
		"testIDs":  opts.TestIDs,
		"refHash":  pbutil.SourceRefHash(opts.SourceRef),
		"subRealm": opts.SubRealm,
	})
	var b spanutil.Buffer
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		tmd := &TestMetadataRow{}
		var compressedTestMetadata spanutil.Compressed
		var compressedSourceRef spanutil.Compressed
		if err := b.FromSpanner(row,
			&tmd.Project,
			&tmd.TestID,
			&tmd.RefHash,
			&tmd.SubRealm,
			&tmd.LastUpdated,
			&compressedSourceRef,
			&compressedTestMetadata,
			&tmd.Position); err != nil {
			return err
		}
		tmd.TestMetadata = &pb.TestMetadata{}
		if err := proto.Unmarshal(compressedTestMetadata, tmd.TestMetadata); err != nil {
			return err
		}
		tmd.SourceRef = &pb.SourceRef{}
		if err := proto.Unmarshal(compressedSourceRef, tmd.SourceRef); err != nil {
			return err
		}
		return f(tmd)
	})
}
