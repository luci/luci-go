// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastore

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/luci/gae/service/blobstore"
	. "github.com/smartystreets/goconvey/convey"
)

func mps(vals ...interface{}) PropertySlice {
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
	{PropertyMap{"Something": {}}, 9},
	{PropertyMap{"Something": mps(100)}, 18},
	{PropertyMap{"Something": mps(100.1, "sup")}, 22},
	{PropertyMap{
		"Something": mps(100, "sup"),
		"Keys":      mps(MakeKey("aid", "ns", "parent", "something", "kind", int64(20))),
	}, 59},
	{PropertyMap{
		"Null":   mps(nil),
		"Bool":   mps(true, false),
		"GP":     mps(GeoPoint{23.2, 122.1}),
		"bskey":  mps(blobstore.Key("hello")),
		"[]byte": mps([]byte("sup")),
	}, 59},
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
		vals := pm[k]
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

	Convey("Test EstimateSize", t, func() {
		for _, tc := range estimateSizeTests {
			Convey(stablePmString(tc.pm), func() {
				So(tc.pm.EstimateSize(), ShouldEqual, tc.expect)
			})
		}
	})
}
