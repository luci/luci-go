// Copyright 2018 The LUCI Authors.
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

package cipd

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/ensure"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	const iid1 = "11111xXp26LpDqWLjKOUmpGorZXaEJGryJO1-Nkp5t0C"
	const iid2 = "22222xXp26LpDqWLjKOUmpGorZXaEJGryJO1-Nkp5t0C"

	exp := template.Expander{"var": "zzz"}

	file, err := ensure.ParseFile(strings.NewReader(fmt.Sprintf(`
		@Subdir 1
		pkg1/${var} ref1

		@Subdir 2
		pkg1/zzz ref2
		pkg2 %s
	`, iid2)))
	if err != nil {
		panic(err)
	}

	Convey("Happy path", t, func() {
		mc := newMockedClient()
		mc.mockResolve("pkg1/zzz", "ref1", iid1)
		mc.mockResolve("pkg1/zzz", "ref2", iid1) // same iid
		mc.mockPin("pkg2", iid2)

		Convey("With VerifyPresence == true", func() {
			r := Resolver{Client: mc, VerifyPresence: true}

			res, err := r.Resolve(ctx, file, exp)
			So(err, ShouldBeNil)
			So(res.PackagesBySubdir, ShouldResemble, common.PinSliceBySubdir{
				"1": common.PinSlice{
					{"pkg1/zzz", iid1},
				},
				"2": common.PinSlice{
					{"pkg1/zzz", iid1},
					{"pkg2", iid2},
				},
			})

			So(mc.resolveCalls, ShouldEqual, 2)  // only two refs
			So(mc.describeCalls, ShouldEqual, 2) // refs resolve into 1 iid + pkg2 iid
		})

		Convey("With VerifyPresence == false", func() {
			r := Resolver{Client: mc, VerifyPresence: false}

			res, err := r.Resolve(ctx, file, exp)
			So(err, ShouldBeNil)
			So(res.PackagesBySubdir, ShouldResemble, common.PinSliceBySubdir{
				"1": common.PinSlice{
					{"pkg1/zzz", iid1},
				},
				"2": common.PinSlice{
					{"pkg1/zzz", iid1},
					{"pkg2", iid2},
				},
			})

			So(mc.resolveCalls, ShouldEqual, 2)  // only two refs
			So(mc.describeCalls, ShouldEqual, 0) // nothing is described
		})
	})

	Convey("ResolveVersion error", t, func() {
		mc := newMockedClient()
		mc.mockResolve("pkg1/zzz", "ref1", iid1) // no ref2
		mc.mockPin("pkg2", iid2)

		r := Resolver{Client: mc, VerifyPresence: true}

		res, err := r.Resolve(ctx, file, exp)
		So(res, ShouldBeNil)
		So(err, ShouldErrLike, `failed to resolve pkg1/zzz@ref2 (line 6): no version "ref2"`)
	})

	Convey("DescribeInstance error", t, func() {
		mc := newMockedClient()
		mc.mockResolve("pkg1/zzz", "ref1", iid1)
		mc.mockResolve("pkg1/zzz", "ref2", iid1) // same iid

		r := Resolver{Client: mc, VerifyPresence: true}

		res, err := r.Resolve(ctx, file, exp)
		So(res, ShouldBeNil)
		So(err, ShouldErrLike,
			fmt.Sprintf(`failed to resolve pkg2@%s (line 7): no such instance`, iid2))
	})

	Convey("Many errors", t, func() {
		mc := newMockedClient()
		r := Resolver{Client: mc, VerifyPresence: true}

		res, err := r.Resolve(ctx, file, exp)
		So(res, ShouldBeNil)

		// Errors are sorted by line
		merr := err.(errors.MultiError)
		So(merr, ShouldHaveLength, 3)
		So(merr[0], ShouldErrLike, `(line 3): no version "ref1"`)
		So(merr[1], ShouldErrLike, `(line 6): no version "ref2"`)
		So(merr[2], ShouldErrLike, `(line 7): no such instance`)
	})
}

func TestResolveAllPlatforms(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	const iid1 = "11111xXp26LpDqWLjKOUmpGorZXaEJGryJO1-Nkp5t0C"
	const iid2 = "22222xXp26LpDqWLjKOUmpGorZXaEJGryJO1-Nkp5t0C"
	const iid3 = "33333xXp26LpDqWLjKOUmpGorZXaEJGryJO1-Nkp5t0C"
	const iid4 = "44444xXp26LpDqWLjKOUmpGorZXaEJGryJO1-Nkp5t0C"

	file, err := ensure.ParseFile(strings.NewReader(`
		$VerifiedPlatform windows-amd64 linux-amd64
		pkg1/${platform} latest
		pkg2/${platform} latest
	`))
	if err != nil {
		panic(err)
	}

	Convey("Happy path", t, func() {
		mc := newMockedClient()
		mc.mockResolve("pkg1/windows-amd64", "latest", iid1)
		mc.mockResolve("pkg1/linux-amd64", "latest", iid2)
		mc.mockResolve("pkg2/windows-amd64", "latest", iid3)
		mc.mockResolve("pkg2/linux-amd64", "latest", iid4)

		r := Resolver{Client: mc}

		res, err := r.ResolveAllPlatforms(ctx, file)
		So(err, ShouldBeNil)
		So(res, ShouldHaveLength, 2)
		So(res[template.Platform{"linux", "amd64"}].PackagesBySubdir, ShouldResemble, common.PinSliceBySubdir{
			"": common.PinSlice{
				{"pkg1/linux-amd64", iid2},
				{"pkg2/linux-amd64", iid4},
			},
		})
		So(res[template.Platform{"windows", "amd64"}].PackagesBySubdir, ShouldResemble, common.PinSliceBySubdir{
			"": common.PinSlice{
				{"pkg1/windows-amd64", iid1},
				{"pkg2/windows-amd64", iid3},
			},
		})
	})

	Convey("Unhappy path", t, func() {
		mc := newMockedClient()
		r := Resolver{Client: mc}

		res, err := r.ResolveAllPlatforms(ctx, file)
		So(res, ShouldBeNil)

		// Errors order is stable.
		var lines []string
		for _, err := range err.(errors.MultiError) {
			lines = append(lines, err.Error())
		}
		So(lines, ShouldResemble, []string{
			`when resolving windows-amd64 - failed to resolve pkg1/windows-amd64@latest (line 3): no version "latest"`,
			`when resolving windows-amd64 - failed to resolve pkg2/windows-amd64@latest (line 4): no version "latest"`,
			`when resolving linux-amd64 - failed to resolve pkg1/linux-amd64@latest (line 3): no version "latest"`,
			`when resolving linux-amd64 - failed to resolve pkg2/linux-amd64@latest (line 4): no version "latest"`,
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

type mockedClient struct {
	Client // to implement the rest of Client interface by nil-panicing

	resolved map[string]common.Pin
	pins     map[common.Pin]struct{}

	l             sync.Mutex
	resolveCalls  int
	describeCalls int
}

func newMockedClient() *mockedClient {
	return &mockedClient{
		resolved: map[string]common.Pin{},
		pins:     map[common.Pin]struct{}{},
	}
}

func (mc *mockedClient) mockResolve(pkg, ver, iid string) {
	mc.resolved[pkg+"@"+ver] = common.Pin{pkg, iid}
	mc.mockPin(pkg, iid)
}

func (mc *mockedClient) mockPin(pkg, iid string) {
	mc.pins[common.Pin{pkg, iid}] = struct{}{}
}

func (mc *mockedClient) BeginBatch(ctx context.Context) {}
func (mc *mockedClient) EndBatch(ctx context.Context)   {}

func (mc *mockedClient) ResolveVersion(ctx context.Context, pkg, ver string) (common.Pin, error) {
	mc.l.Lock()
	mc.resolveCalls++
	mc.l.Unlock()
	p, ok := mc.resolved[pkg+"@"+ver]
	if !ok {
		return common.Pin{}, fmt.Errorf("no version %q", ver)
	}
	return p, nil
}

func (mc *mockedClient) DescribeInstance(ctx context.Context, pin common.Pin, opts *DescribeInstanceOpts) (*InstanceDescription, error) {
	mc.l.Lock()
	mc.describeCalls++
	mc.l.Unlock()
	if _, ok := mc.pins[pin]; ok {
		return &InstanceDescription{}, nil
	}
	return nil, fmt.Errorf("no such instance")
}
