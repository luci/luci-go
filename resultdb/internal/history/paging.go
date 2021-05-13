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

package history

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/pagination"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// DefaultPageSize is how many test results to return when unspecified.
const DefaultPageSize = 100

var pageBreak = errors.BoolTag{
	Key: errors.NewTagKey("test results history page break"),
}

// pageToken computes a page token to continue querying at this point.
func pageToken(ip indexPoint, offset int) string {
	return pagination.Token(ip.kind(), ip.tokenField(), fmt.Sprint(offset))
}

// initPaging sets the default page size if needed, parses the token in
// the request and modifies the appropriate range field.
// It returns the offset into the first timestamp/ordinal, i.e. the number of
// results to skip.
// It is assumed that the page token is valid.
func initPaging(in *pb.GetTestResultHistoryRequest) int {
	if in.PageSize == 0 {
		in.PageSize = DefaultPageSize
	}
	if in.PageToken == "" {
		return 0
	}
	ip, offset, _ := parsePageToken(in.PageToken)
	ip.initPaging(in)
	return offset
}

func parsePageToken(t string) (indexPoint, int, error) {
	switch parts, err := pagination.ParseToken(t); {
	case err != nil:
		return nil, 0, err
	case len(parts) != 3:
		return nil, 0, pagination.InvalidToken(errors.Reason("expected 3 parts, got %d", len(parts)).Err())
	default:
		ip, err := indexPointFromTokenField(parts[0], parts[1])
		if err != nil {
			return nil, 0, pagination.InvalidToken(err)
		}
		offset, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, 0, pagination.InvalidToken(err)
		}
		return ip, offset, nil
	}
}

// ValidatePageToken returns an error if the given token is invalid.
func ValidatePageToken(t string) error {
	_, _, err := parsePageToken(t)
	return err
}

// indexPoint is an interface that should be implemented by structs that
// represent a point along the history index such as a timestamp or commit
// position.
type indexPoint interface {
	kind() string
	tokenField() string
	initPaging(*pb.GetTestResultHistoryRequest)
	initEntry(*pb.GetTestResultHistoryResponse_Entry)
}

func indexPointFromTokenField(kind, val string) (indexPoint, error) {
	switch kind {
	case "ts":
		return newTSIndexPoint(val)
	default:
		return nil, errors.Reason("unknown index point kind: %q", kind).Err()
	}
}

type tsIndexPoint timestamppb.Timestamp

func (ts *tsIndexPoint) kind() string {
	return "ts"
}

func (ts *tsIndexPoint) tokenField() string {
	b, err := proto.Marshal((*timestamppb.Timestamp)(ts))
	if err != nil {
		panic("impossible marshaling error")
	}
	return hex.EncodeToString(b)
}

func (ts *tsIndexPoint) initPaging(in *pb.GetTestResultHistoryRequest) {
	in.GetTimeRange().Latest = (*timestamppb.Timestamp)(ts)
}

func (ts *tsIndexPoint) initEntry(e *pb.GetTestResultHistoryResponse_Entry) {
	e.InvocationTimestamp = (*timestamppb.Timestamp)(ts)
}

func newTSIndexPoint(s string) (*tsIndexPoint, error) {
	var ret *timestamppb.Timestamp
	b, err := hex.DecodeString(s)
	if err == nil {
		ret = &timestamppb.Timestamp{}
		err = proto.Unmarshal(b, ret)
	}
	return (*tsIndexPoint)(ret), err
}

// sortEntries sorts the items in the slice by (TestId, VariantHash,
// ResultId).
func sortEntries(s []*pb.GetTestResultHistoryResponse_Entry) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].Result.TestId != s[j].Result.TestId {
			return s[i].Result.TestId < s[j].Result.TestId
		}
		if s[i].Result.VariantHash != s[j].Result.VariantHash {
			return s[i].Result.VariantHash < s[j].Result.VariantHash
		}
		return s[i].Result.ResultId < s[j].Result.ResultId
	})
}
