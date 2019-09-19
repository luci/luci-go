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

	"go.chromium.org/luci/common/errors"

	resultspb "go.chromium.org/luci/results/proto/v1"
)

// ErrBadFormat is returned when the unmarshaled JSON doesn't match the format.
//
// This means marshalling is successful, but the resulting struct's fields don't
// match those expected in the parsing format.
var ErrBadFormat = errors.New("JSON doesn't match expected parsing format")

// getVariantID returns an ID for the provided variant.
func getVariantID(variant map[string]string) string {
	// TODO: Implement.
	return "variant ID"
}

func getVariantFromDefMap(def map[string]string, sortedKeys []string) *resultspb.VariantDef {
	// If we didn't build up a sorted keys list already, do so.
	if len(sortedKeys) <= 0 {
		for k := range def {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)
	}

	// TODO: Handle too many pairs/too long keys.

	pairs := make([]string, 0, len(def))
	for _, k := range sortedKeys {
		pairs = append(pairs, fmt.Sprintf("%s:%s", k, def[k]))
	}

	h := sha256.New()
	h.Write([]byte(strings.Join(pairs, "\n")))
	digest := string(h.Sum(nil))

	return &resultspb.VariantDef{
		Def:    def,
		Digest: digest,
	}
}

// secondsToTimestamp converts a UTC float64 timestamp to a ptypes Timestamp.
func secondsToTimestamp(t float64) *timestamp.Timestamp {
	if t < 0 {
		panic("negative time given in SecondsSinceEpoch")
	}

	return &timestamp.Timestamp{
		Seconds: int64(t),
		Nanos:   int32(math.Pow10(9) * (t - float64(int64(t)))),
	}
}
