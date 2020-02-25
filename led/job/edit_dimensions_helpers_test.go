// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
	"sort"

	. "github.com/smartystreets/goconvey/convey"
)

func mustGetDimensions(jd *Definition) ExpiringDimensions {
	ret, err := jd.Info().Dimensions()
	So(err, ShouldBeNil)
	return ret
}

// baselineDims sets some baseline dimensions on jd and returns the
// ExpiringDimensions which Info().GetDimensions() should now return.
func baselineDims(jd *Definition) ExpiringDimensions {
	toSet := ExpiringDimensions{
		"key": []ExpiringValue{
			{Value: "B", Expiration: swSlice2Exp},
			{Value: "Z"},
			{Value: "A", Expiration: swSlice1Exp},
			{Value: "AA", Expiration: swSlice1Exp},
			{Value: "C", Expiration: swSlice3Exp},
		},
	}

	SoEdit(jd, func(je Editor) {
		// set in a non-sorted order
		je.SetDimensions(toSet)
	})

	// Info().GetDimensions() always returns filled expirations.
	toSet["key"][1].Expiration = swSlice3Exp

	sort.Slice(toSet["key"], func(i, j int) bool {
		return toSet["key"][i].Value < toSet["key"][j].Value
	})

	return toSet
}
