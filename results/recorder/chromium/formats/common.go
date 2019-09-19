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

// GetID returns a SHA25 hash of newline-joined "<key>:<val>" strings from the variant as an ID.
func (d VariantDefMap) GetID() string {
	sortedKeys := d.getSortedKeys()

	// TODO: Verify input key/values.
	// TODO: Handle too many pairs/too long keys.

	h := sha256.New()
	for i, k := range sortedKeys {
		h.Write([]byte(k))
		h.Write([]byte(":"))
		h.Write([]byte(d[k]))
		if i < len(sortedKeys)-1 {
			h.Write([]byte("\n"))
		}
	}

	return string(h.Sum(nil))
}

// GetProto converts the VariantDefMap to a fully fledged resultspb.VariantDef proto.
func (d VariantDefMap) GetProto() *resultspb.VariantDef {
	return &resultspb.VariantDef{
		Def:    d,
		Digest: d.GetID(),
	}
}

func (d VariantDefMap) getSortedKeys() []string {
	var keys []string
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
