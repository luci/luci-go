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
		otherTag := &vpython.Pep425Tag{Version: "otherVersion", Abi: "otherABI", Arch: "otherArch"}
		maybeTag := &vpython.Pep425Tag{Version: "maybeVersion", Abi: "maybeABI", Arch: "maybeArch"}

		pkgFoo := &vpython.Spec_Package{Name: "foo", Version: "1"}
		pkgBar := &vpython.Spec_Package{Name: "bar", Version: "2"}
		pkgBaz := &vpython.Spec_Package{Name: "baz", Version: "3"}
		pkgMaybe := &vpython.Spec_Package{Name: "maybe", Version: "3", MatchTag: []*vpython.Pep425Tag{
			{Version: maybeTag.Version},
		}}

		env := vpython.Environment{
			Pep425Tag: []*vpython.Pep425Tag{otherTag},
		}
		var rt vpython.Runtime

		Convey(`Will normalize an empty spec`, func() {
			So(NormalizeEnvironment(&env), ShouldBeNil)
			So(env, ShouldResemble, vpython.Environment{
				Spec:      &vpython.Spec{},
				Runtime:   &vpython.Runtime{},
				Pep425Tag: []*vpython.Pep425Tag{otherTag},
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

				So(Hash(env.Spec, &rt, ""), ShouldEqual, "7e80b8643051ce0d82bf44fb180687e988791cfd7f3da39861370f0a56fc80f8")
				So(Hash(env.Spec, &rt, "extra"), ShouldEqual, "140a02bb88b011d4aceafb9533266288fd4b441c3bdb70494419b3ef76457f34")
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
