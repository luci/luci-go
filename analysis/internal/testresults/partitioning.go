// Copyright 2025 The LUCI Authors.
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

package testresults

import (
	"time"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

// TimeRange represents a time range with two bounds.
type TimeRange struct {
	// Earliest time (inclusive) to include in the range.
	Earliest time.Time
	// Latest time (exclusive) to include in the range.
	Latest time.Time
}

// Intersect returns a time range which contains the intersection
// of two time ranges. This result may be empty (earliest equal to or after latest).
func (d TimeRange) Intersect(other TimeRange) TimeRange {
	// The intersection of two date ranges is the maximum of their earliest
	// dates and the minimum of their latest dates.
	earliest := d.Earliest
	if other.Earliest.After(earliest) {
		earliest = other.Earliest
	}
	latest := d.Latest
	if other.Latest.Before(latest) {
		latest = other.Latest
	}
	return TimeRange{
		Earliest: earliest,
		Latest:   latest,
	}
}

// Partition creates a partitioner for the time range.
func (d TimeRange) Partition() *TimeRangePartitioner {
	return &TimeRangePartitioner{
		entire: d,
		next:   d.Latest,
	}
}

// Days returns the number of fractional days in the queried interval.
func (d TimeRange) Days() float64 {
	if d.Latest.Before(d.Earliest) {
		// Empty interval.
		return 0
	}
	return d.Latest.Sub(d.Earliest).Hours() / 24
}

// TimeRangePartitioner provides methods to partition a time range
// into a series of non-overlapping ranges.
//
// The partitions proceeds backwards from the last time in the range
// towards the earliest time.
type TimeRangePartitioner struct {
	entire TimeRange
	// next is the latest time (exclusive) to be included in the next partition.
	//
	// If next is before or equal to entire.Earliest, we have returned
	// all partitions.
	next time.Time
}

// Next returns the next partition of the interval, which contains
// `days` UTC days. The partition will be cut on a UTC day boundary.
//
// If the entire interval does not end on a UTC day boundary, and
// this is the first or last partition, the interval may contain
// fractional days.
func (p *TimeRangePartitioner) Next(days int) (tr TimeRange, ok bool) {
	if !p.next.After(p.entire.Earliest) {
		return TimeRange{}, false
	}
	current := p.next.UTC()
	next := current.Add(time.Duration(-days) * 24 * time.Hour).UTC()

	// Make `next` fall on a day boundary to ensure all split
	// points other than the Earliest and Latest times are on a day
	// boundary.
	if !next.Truncate(24 * time.Hour).Equal(next) {
		// Round back up to the next day.
		next = next.Truncate(24 * time.Hour)
		next = next.Add(24 * time.Hour)
	}
	if next.Before(p.entire.Earliest) {
		next = p.entire.Earliest.UTC()
	}
	p.next = next
	return TimeRange{Earliest: next, Latest: current}, true
}

// TimeRangeFromProto converts a *pb.TimeRange to a TimeRange.
func TimeRangeFromProto(tr *pb.TimeRange) TimeRange {
	var afterTime time.Time
	if tr.GetEarliest() != nil {
		afterTime = tr.GetEarliest().AsTime()
	} else {
		afterTime = MinSpannerTimestamp
	}

	var beforeTime time.Time
	if tr.GetLatest() != nil {
		beforeTime = tr.GetLatest().AsTime()
	} else {
		beforeTime = MaxSpannerTimestamp
	}
	return TimeRange{
		Earliest: afterTime.UTC(),
		Latest:   beforeTime.UTC(),
	}
}
