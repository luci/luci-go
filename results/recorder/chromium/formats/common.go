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
	"crypto/sha256"
	"encoding/hex"
	"io"
	"math"
	"sort"

	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"

	resultspb "go.chromium.org/luci/results/proto/v1"
)

// VariantDefMap contains the key:val pairs that define a Variant.
//
// It is the "Def" part of a VariantDef proto.
type VariantDefMap map[string]string

// ID returns a hex SHA256 hash of newline-joined "<key>:<val>" strings from the variant as an ID.
func (d VariantDefMap) ID() string {
	sortedKeys := d.sortedKeys()

	// TODO: Verify input key/values.
	// TODO: Handle too many pairs/too long keys.

	h := sha256.New()
	for _, k := range sortedKeys {
		io.WriteString(h, k)
		io.WriteString(h, ":")
		io.WriteString(h, d[k])
		io.WriteString(h, "\n")
	}

	return hex.EncodeToString(h.Sum(nil))
}

// Proto converts the VariantDefMap to a fully fledged resultspb.VariantDef proto.
func (d VariantDefMap) Proto() *resultspb.VariantDef {
	return &resultspb.VariantDef{
		Def:    d,
		Digest: d.ID(),
	}
}

func (d VariantDefMap) sortedKeys() []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// testVariant gets the test variant def from merging the input maps.
//
// If multiple maps define the same key, the last one wins.
func MergeTestVariantMaps(maps ...VariantDefMap) VariantDefMap {
	def := VariantDefMap{}
	for _, m := range maps {
		for k, v := range m {
			def[k] = v
		}
	}
	return def
}

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
