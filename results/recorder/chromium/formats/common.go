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

package formats

import (
	"math"
	"sort"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"

	resultspb "go.chromium.org/luci/results/proto/v1"
)

// SortTagsInPlace sorts in-place the tags slice lexicographically by key, then value.
func SortTagsInPlace(tags []*resultspb.StringPair) {
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Key != tags[j].Key {
			return tags[i].Key < tags[j].Key
		}
		return tags[i].Value < tags[j].Value
	})
}

// secondsToTimestamp converts a UTC float64 timestamp to a ptypes Timestamp.
func secondsToTimestamp(t float64) *timestamp.Timestamp {
	if t < 0 {
		panic("negative time given in secondsToTimestamp")
	}
	seconds, nanos := splitSeconds(t)
	return &timestamp.Timestamp{Seconds: seconds, Nanos: nanos}
}

func secondsToDuration(t float64) *duration.Duration {
	seconds, nanos := splitSeconds(t)
	return &duration.Duration{Seconds: seconds, Nanos: nanos}
}

func splitSeconds(t float64) (seconds int64, nanos int32) {
	seconds = int64(t)
	nanos = int32(1e9 * (t - math.Floor(t)))
	return
}
