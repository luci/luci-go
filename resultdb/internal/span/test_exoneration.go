// Copyright 2019 The LUCI Authors.
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

package span

import (
	"context"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// MustParseTestExonerationName extracts invocation, test path and exoneration
// IDs from the name.
// Panics on failure.
func MustParseTestExonerationName(name string) (invID InvocationID, testPath, exonerationID string) {
	invIDStr, testPath, exonerationID, err := pbutil.ParseTestExonerationName(name)
	if err != nil {
		panic(err)
	}
	invID = InvocationID(invIDStr)
	return
}

// ReadTestExonerationFull reads a test exoneration from Spanner.
// If it does not exist, the returned error is annotated with NotFound GRPC
// code.
func ReadTestExonerationFull(ctx context.Context, txn Txn, name string) (*pb.TestExoneration, error) {
	invIDStr, testPath, exonerationID, err := pbutil.ParseTestExonerationName(name)
	if err != nil {
		return nil, err
	}
	invID := InvocationID(invIDStr)

	ret := &pb.TestExoneration{
		Name:          name,
		TestPath:      testPath,
		ExonerationId: exonerationID,
	}

	// Populate fields from TestExonerations table.
	err = ReadRow(ctx, txn, "TestExonerations", invID.Key(testPath, exonerationID), map[string]interface{}{
		"Variant":             &ret.Variant,
		"ExplanationMarkdown": &ret.ExplanationMarkdown,
	})
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return nil, errors.Reason("%q not found", ret.Name).
			InternalReason("%s", err).
			Tag(grpcutil.NotFoundTag).
			Err()

	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch %q", ret.Name).Err()

	default:
		return ret, nil
	}
}

// ReadTestExonerations reads all test exonerations from the invocation.
func ReadTestExonerations(ctx context.Context, txn Txn, invID InvocationID, cursorTok string, pageSize int) (tes []*pb.TestExoneration, nextCursorTok string, err error) {
	if pageSize <= 0 {
		panic("pageSize must be positive")
	}

	// Set start position if requested.
	keyRange := invID.Key().AsPrefix()
	switch pos, tokErr := pagination.ParseToken(cursorTok); {
	case tokErr != nil:
		err = errors.Reason("invalid page_token").
			InternalReason("%s", tokErr).
			Tag(grpcutil.InvalidArgumentTag).Err()
		return

	case pos == nil:
		break

	case len(pos) == 2:
		// Start after the cursor position.
		keyRange.Kind = spanner.OpenClosed
		keyRange.Start = invID.Key(pos[0], pos[1])

	default:
		err = errors.Reason("invalid page_token").
			InternalReason("unexpected string slice %q for TestResults cursor position", pos).
			Tag(grpcutil.InvalidArgumentTag).Err()
		return
	}

	columns := []string{"TestPath", "ExonerationId", "Variant", "ExplanationMarkdown"}
	opts := &spanner.ReadOptions{Limit: pageSize}
	var b Buffer
	err = txn.ReadWithOptions(ctx, "TestExonerations", keyRange, columns, opts).Do(func(row *spanner.Row) error {
		ex := &pb.TestExoneration{}
		err := b.FromSpanner(row, &ex.TestPath, &ex.ExonerationId, &ex.Variant, &ex.ExplanationMarkdown)
		if err != nil {
			return err
		}
		ex.Name = pbutil.TestExonerationName(string(invID), ex.TestPath, ex.ExonerationId)
		tes = append(tes, ex)
		return nil
	})
	if err != nil {
		tes = nil
		return
	}

	if len(tes) == pageSize {
		last := tes[pageSize-1]
		nextCursorTok = pagination.Token(last.TestPath, last.ExonerationId)
	}
	return
}
