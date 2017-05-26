// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package spec

import (
	"testing"

	"github.com/luci/luci-go/vpython/api/vpython"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNormalizeAndHash(t *testing.T) {
	t.Parallel()

	pkgFoo := &vpython.Spec_Package{Name: "foo", Version: "1"}
	pkgBar := &vpython.Spec_Package{Name: "bar", Version: "2"}
	pkgBaz := &vpython.Spec_Package{Name: "baz", Version: "3"}

	Convey(`Test manifest generation`, t, func() {
		var spec vpython.Spec
		var rt vpython.Runtime

		Convey(`Will normalize an empty spec`, func() {
			So(Normalize(&spec, pkgFoo), ShouldBeNil)
			So(spec, ShouldResemble, vpython.Spec{
				Virtualenv: pkgFoo,
			})
		})

		Convey(`Will normalize to sorted order.`, func() {
			spec.Wheel = []*vpython.Spec_Package{pkgFoo, pkgBar, pkgBaz}
			So(Normalize(&spec, nil), ShouldBeNil)
			So(spec, ShouldResemble, vpython.Spec{
				Wheel: []*vpython.Spec_Package{pkgBar, pkgBaz, pkgFoo},
			})

			So(Hash(&spec, &rt, ""), ShouldEqual, "7e80b8643051ce0d82bf44fb180687e988791cfd7f3da39861370f0a56fc80f8")
			So(Hash(&spec, &rt, "extra"), ShouldEqual, "140a02bb88b011d4aceafb9533266288fd4b441c3bdb70494419b3ef76457f34")
		})

		Convey(`Will fail to normalize if there are duplicate wheels.`, func() {
			spec.Wheel = []*vpython.Spec_Package{pkgFoo, pkgFoo, pkgBar, pkgBaz}
			So(Normalize(&spec, nil), ShouldErrLike, "duplicate spec entries")

			// Even if the versions differ.
			fooClone := *pkgFoo
			fooClone.Version = "other"
			spec.Wheel = []*vpython.Spec_Package{pkgFoo, &fooClone, pkgBar, pkgBaz}
			So(Normalize(&spec, nil), ShouldErrLike, "duplicate spec entries")
		})
	})
}
