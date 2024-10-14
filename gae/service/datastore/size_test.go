// Copyright 2015 The LUCI Authors.
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

package datastore

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/blobstore"
)

func mps(vals ...any) PropertySlice {
	ret := make(PropertySlice, len(vals))
	for i, val := range vals {
		ret[i] = mp(val)
	}
	return ret
}

var estimateSizeTests = []struct {
	pm     PropertyMap
	expect int
}{
	{PropertyMap{"Something": mps()}, 9},
	{PropertyMap{"Something": mps(100)}, 18},
	{PropertyMap{"Something": mps(100.1, "sup")}, 22},
	{PropertyMap{
		"Something": mps(100, "sup"),
		"Keys":      mps(MkKeyContext("aid", "ns").MakeKey("parent", "something", "kind", int64(20))),
	}, 59},
	{PropertyMap{
		"Null":   mps(nil),
		"Bool":   mps(true, false),
		"GP":     mps(GeoPoint{23.2, 122.1}),
		"bskey":  mps(blobstore.Key("hello")),
		"[]byte": mps([]byte("sup")),
		"Inner": mp(PropertyMap{
			"$id":   mp(12),
			"$kind": mp("Kind"),
		}),
	}, 77},
}

func stablePmString(pm PropertyMap) string {
	keys := make([]string, 0, len(pm))
	for k := range pm {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buf := &bytes.Buffer{}
	_, _ = buf.WriteString("map[")
	for i, k := range keys {
		if i != 0 {
			_, _ = buf.WriteString(" ")
		}
		vals := pm.Slice(k)
		strs := make([]string, len(vals))
		for i, v := range vals {
			strs[i] = v.GQL()
		}
		fmt.Fprintf(buf, "%s:[%s]", k, strings.Join(strs, ", "))
	}
	_, _ = buf.WriteRune(']')
	return buf.String()
}

func TestEstimateSizes(t *testing.T) {
	t.Parallel()

	ftt.Run("Test EstimateSize", t, func(t *ftt.Test) {
		for _, tc := range estimateSizeTests {
			t.Run(stablePmString(tc.pm), func(t *ftt.Test) {
				assert.Loosely(t, tc.pm.EstimateSize(), should.Equal(tc.expect))
			})
		}
	})
}
