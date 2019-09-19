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
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/golang/protobuf/ptypes/timestamp"

	resultspb "go.chromium.org/luci/results/proto/v1"
)


// VariantDefMap contains the key:val pairs that define a Variant.
//
// It is the "Def" part of a VariantDef proto.
type VariantDefMap map[string]string

// GetID returns an ID for the provided variant.
func (d VariantDefMap) GetID() string {
	// TODO: Implement.
	return "variant ID"
}

// GetProto converts the VariantDefMap to a fully fledged resultspb.VariantDef proto.
func (d VariantDefMap) GetProto(sortedKeys []string) *resultspb.VariantDef {
	// If we didn't build up a sorted key list already, do so.
	if len(sortedKeys) <= 0 {
		for k := range d {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)
	}

	// TODO: Handle too many pairs/too long keys.

	pairs := make([]string, len(d))
	for i, k := range sortedKeys {
		pairs[i] = fmt.Sprintf("%s:%s", k, d[k])
	}

	h := sha256.New()
	h.Write([]byte(strings.Join(pairs, "\n")))
	digest := string(h.Sum(nil))

	return &resultspb.VariantDef{
		Def:    d,
		Digest: digest,
	}
}

// secondsToTimestamp converts a UTC float64 timestamp to a ptypes Timestamp.
func secondsToTimestamp(t float64) *timestamp.Timestamp {
	if t < 0 {
		panic("negative time given in secondsToTimestamp")
	}

	return &timestamp.Timestamp{
		Seconds: int64(t),
		Nanos:   int32(1e9 * (t - math.Floor(t))),
	}
}
