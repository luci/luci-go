// Copyright 2017 The LUCI Authors.
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
		pkgFooV2 := &vpython.Spec_Package{Name: "foo", Version: "2"}
		pkgBar := &vpython.Spec_Package{Name: "bar", Version: "2"}
		pkgBaz := &vpython.Spec_Package{Name: "baz", Version: "3"}

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

			Convey(`With a match entry, will match tags`, func() {
				pkgMaybe := &vpython.Spec_Package{Name: "maybe", Version: "3", MatchTag: []*vpython.PEP425Tag{
					{Python: maybeTag.Python},
				}}
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

			Convey(`With multiple match entries, will match tags`, func() {
				pkgMaybe := &vpython.Spec_Package{Name: "maybe", Version: "3", MatchTag: []*vpython.PEP425Tag{
					{Python: maybeTag.Python},
				}}
				pkgMaybeNonexistTag := &vpython.Spec_Package{Name: "maybe", Version: "3", MatchTag: []*vpython.PEP425Tag{
					{Python: "nonexist"},
				}}
				env.Spec.Wheel = []*vpython.Spec_Package{pkgMaybe, pkgFoo, pkgMaybeNonexistTag}
				env.Pep425Tag = append(env.Pep425Tag, maybeTag)

				So(NormalizeEnvironment(&env), ShouldBeNil)

				pkgMaybe.MatchTag = nil
				So(env.Spec, ShouldResemble, &vpython.Spec{
					Wheel: []*vpython.Spec_Package{pkgFoo, pkgMaybe},
				})
			})

			Convey(`With one absolute and one match, will always match`, func() {
				pkgAlways := &vpython.Spec_Package{Name: "maybe", Version: "3"}
				pkgMaybeNonexistTag := &vpython.Spec_Package{Name: "maybe", Version: "3", MatchTag: []*vpython.PEP425Tag{
					{Python: "nonexist"},
				}}
				env.Spec.Wheel = []*vpython.Spec_Package{pkgFoo, pkgMaybeNonexistTag}
				env.Pep425Tag = append(env.Pep425Tag, maybeTag)

				So(NormalizeEnvironment(&env), ShouldBeNil)
				So(env.Spec, ShouldResemble, &vpython.Spec{
					Wheel: []*vpython.Spec_Package{pkgFoo},
				})

				env.Spec.Wheel = []*vpython.Spec_Package{pkgAlways, pkgFoo, pkgMaybeNonexistTag}

				So(NormalizeEnvironment(&env), ShouldBeNil)
				So(env.Spec, ShouldResemble, &vpython.Spec{
					Wheel: []*vpython.Spec_Package{pkgFoo, pkgAlways},
				})
			})

			Convey(`Will normalize if there are duplicate wheels that share a version.`, func() {
				env.Spec.Wheel = []*vpython.Spec_Package{pkgFoo, pkgFoo, pkgBar, pkgBaz}
				So(NormalizeEnvironment(&env), ShouldBeNil)
				So(env.Spec, ShouldResemble, &vpython.Spec{
					Wheel: []*vpython.Spec_Package{pkgBar, pkgBaz, pkgFoo},
				})
			})

			Convey(`Will fail to normalize if there are duplicate wheels with different versions.`, func() {
				env.Spec.Wheel = []*vpython.Spec_Package{pkgFoo, pkgFooV2, pkgFoo, pkgBar, pkgBaz}
				So(NormalizeEnvironment(&env), ShouldErrLike, "multiple versions for package")
			})
		})
	})
}
