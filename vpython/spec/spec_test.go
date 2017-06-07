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

	Convey(`Test manifest generation`, t, func() {
		otherTag := &vpython.PEP425Tag{Python: "otherPython", Abi: "otherABI", Platform: "otherPlatform"}
		maybeTag := &vpython.PEP425Tag{Python: "maybePython", Abi: "maybeABI", Platform: "maybePlatform"}

		pkgFoo := &vpython.Spec_Package{Name: "foo", Version: "1"}
		pkgBar := &vpython.Spec_Package{Name: "bar", Version: "2"}
		pkgBaz := &vpython.Spec_Package{Name: "baz", Version: "3"}
		pkgMaybe := &vpython.Spec_Package{Name: "maybe", Version: "3", MatchTag: []*vpython.PEP425Tag{
			{Python: maybeTag.Python},
		}}

		env := vpython.Environment{
			Pep425Tag: []*vpython.PEP425Tag{otherTag},
		}
		var rt vpython.Runtime

		Convey(`Will normalize an empty spec`, func() {
			So(NormalizeEnvironment(&env), ShouldBeNil)
			So(env, ShouldResemble, vpython.Environment{
				Spec:      &vpython.Spec{},
				Runtime:   &vpython.Runtime{},
				Pep425Tag: []*vpython.PEP425Tag{otherTag},
			})
		})

		Convey(`With a non-nil spec`, func() {
			env.Spec = &vpython.Spec{}

			Convey(`Will normalize to sorted order.`, func() {
				env.Spec.Wheel = []*vpython.Spec_Package{pkgFoo, pkgBar, pkgBaz}
				So(NormalizeEnvironment(&env), ShouldBeNil)
				So(env.Spec, ShouldResemble, &vpython.Spec{
					Wheel: []*vpython.Spec_Package{pkgBar, pkgBaz, pkgFoo},
				})

				So(Hash(env.Spec, &rt, ""), ShouldEqual, "1e32c02610b51f8c3807203fccd3e8d01d252868d52eb4ee9df135ef6533c5ae")
				So(Hash(env.Spec, &rt, "extra"), ShouldEqual, "d047eb021f50534c050aaa10c70dc7b4a9b511fab00cf67a191b2b0805f24420")
			})

			Convey(`With match tags`, func() {
				env.Spec.Wheel = []*vpython.Spec_Package{pkgFoo, pkgMaybe}

				Convey(`Will omit the package if it doesn't match a tag`, func() {
					So(NormalizeEnvironment(&env), ShouldBeNil)
					So(env.Spec, ShouldResemble, &vpython.Spec{
						Wheel: []*vpython.Spec_Package{pkgFoo},
					})
				})

				Convey(`Will include the package if it matches a tag, and strip the match field.`, func() {
					env.Pep425Tag = append(env.Pep425Tag, maybeTag)

					So(NormalizeEnvironment(&env), ShouldBeNil)

					pkgMaybe.MatchTag = nil
					So(env.Spec, ShouldResemble, &vpython.Spec{
						Wheel: []*vpython.Spec_Package{pkgFoo, pkgMaybe},
					})
				})
			})

			Convey(`Will fail to normalize if there are duplicate wheels.`, func() {
				env.Spec.Wheel = []*vpython.Spec_Package{pkgFoo, pkgFoo, pkgBar, pkgBaz}
				So(NormalizeEnvironment(&env), ShouldErrLike, "duplicate spec entries")

				// Even if the versions differ.
				fooClone := *pkgFoo
				fooClone.Version = "other"
				env.Spec.Wheel = []*vpython.Spec_Package{pkgFoo, &fooClone, pkgBar, pkgBaz}
				So(NormalizeEnvironment(&env), ShouldErrLike, "duplicate spec entries")
			})
		})
	})
}
