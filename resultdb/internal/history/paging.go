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
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/pagination"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// DefaultPageSize is how many results to return when unspecified.
	DefaultPageSize = 100
)

// PageItem is used by the result workers to communicate to the collector
// either a result, or a "page break" i.e. a signal that we are done streaming
// results in the current timstamp/ordinal.
type PageItem struct {
	Entry     *pb.GetTestResultHistoryResponse_Entry
	PageBreak IndexPoint
}

// ExpirationFlag returns a pointer to a bool that will be set to true when
// the context is close to expiration.
func ExpirationFlag(ctx context.Context) *bool {
	gracePeriod := 5 * time.Second
	outOfTime := false
	dl, ok := ctx.Deadline()
	if ok {
		go func(t time.Time) {
			d := time.Until(t)
			if d < 0 {
				outOfTime = true
				return
			}
			select {
			case <-ctx.Done():
			case <-time.After(d):
				outOfTime = true
			}
		}(dl.Add(-gracePeriod))
	}
	return &outOfTime
}

func MakePageToken(indexPoint IndexPoint, offset int) string {
	return pagination.Token(indexPoint.toTokenField(), fmt.Sprint(offset))
}

// InitPaging sets the default page size if needed, parses the token in
// the request and modifies the the appropriate range.
// It returns the offset into the first timestamp/ordinal, i.e. the number of
// results to skip.
// It is assumed that the incoming request including the page token is valid.
func InitPaging(in *pb.GetTestResultHistoryRequest) int {
	if in.PageSize == 0 {
		in.PageSize = DefaultPageSize
	}
	if in.PageToken == "" {
		return 0
	}
	parts, _ := pagination.ParseToken(in.PageToken)
	startIP, _ := indexPointFromTokenField(parts[0])
	skipResults, _ := strconv.Atoi(parts[1])
	switch startIP.(type) {
	case *tsIndexPoint:
		in.GetTimeRange().Latest = &timestamp.Timestamp{
			Seconds: startIP.(*tsIndexPoint).Seconds,
			Nanos:   startIP.(*tsIndexPoint).Nanos,
		}
	default:
		// Should have been validated.
		panic("Unsupported page token contents")
	}
	return skipResults
}

//ValidatePageToken returns an error if the given token is invalid.
func ValidatePageToken(t string) error {
	parts, err := pagination.ParseToken(t)
	switch {
	case err != nil:
		return err
	case len(parts) != 2:
		return errors.Reason("invalid page token").Err()
	default:
		if _, err := indexPointFromTokenField(parts[0]); err != nil {
			return errors.Reason("invalid page token").Err()
		}
		if _, err := strconv.Atoi(parts[1]); err != nil {
			return errors.Reason("invalid page token").Err()
		}
	}
	return nil
}

func indexPointFromTokenField(v string) (IndexPoint, error) {
	fields := strings.SplitN(v, ":", 2)
	if len(fields) != 2 {
		return nil, errors.Reason("Invalid index point string: %s", v).Err()
	}
	if fields[0] == "ts" {
		return newTSIndexPoint(fields[1])
	}
	return nil, errors.Reason("Unknown index point kind: %s", fields[0]).Err()
}

// IndexPoint is an interface that should be implemented by structs that
// represent a point along the history index such as a timestamp or commit
// position.
type IndexPoint interface {
	toTokenField() string
}

type tsIndexPoint timestamp.Timestamp

func (ts *tsIndexPoint) asString() string {
	return fmt.Sprintf("%d.%d", ts.Seconds, ts.Nanos)
}

func (ts *tsIndexPoint) toTokenField() string {
	return fmt.Sprintf("ts:%s", ts.asString())
}

func newTSIndexPoint(s string) (*tsIndexPoint, error) {
	var err error
	var secs, nanos int64

	fields := strings.SplitN(s, ".", 2)
	if len(fields) != 2 {
		err = errors.Reason("Bad format for tsIndexPoint string: %q, expected <int64>.<int32>", s).Err()
	}
	if err == nil {
		secs, err = strconv.ParseInt(fields[0], 10, 64)
	}
	if err == nil {
		nanos, err = strconv.ParseInt(fields[1], 10, 32)
	}
	return (*tsIndexPoint)(&timestamp.Timestamp{
		Seconds: secs,
		Nanos:   int32(nanos),
	}), err

}
