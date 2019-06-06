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

package cli

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"go.chromium.org/luci/common/data/stringset"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestPrintAndDone(t *testing.T) {
	t.Parallel()

	Convey("printAndDone", t, func(c C) {
		ctx := context.Background()

		call := func(args []string, fn func(string) (*pb.Build, error)) []buildResult {
			run := &printRun{id: true}
			resultC := make(chan buildResult)
			go func() {
				run.runOrdered(ctx, 16, argChan(args), resultC, func(ctx context.Context, arg string) (*pb.Build, error) {
					return fn(arg)
				})
				close(resultC)
			}()

			var ret []buildResult
			for r := range resultC {
				ret = append(ret, r)
			}
			return ret
		}

		Convey("actual args", func() {
			var m sync.Mutex
			actualArgs := stringset.New(3)
			call([]string{"1", "2"}, func(arg string) (*pb.Build, error) {
				m.Lock()
				defer m.Unlock()
				actualArgs.Add(arg)
				return &pb.Build{SummaryMarkdown: arg}, nil
			})
			So(actualArgs, ShouldResemble, stringset.NewFromSlice("1", "2"))
		})

		Convey("one build", func() {
			build := &pb.Build{SummaryMarkdown: "1"}
			res := call([]string{"1"}, func(arg string) (*pb.Build, error) {
				return build, nil
			})
			So(res[0].arg, ShouldEqual, "1")
			So(res[0].err, ShouldBeNil)
			So(res[0].build, ShouldResembleProto, build)
		})

		Convey("two builds", func() {
			build := &pb.Build{SummaryMarkdown: "1"}
			res := call([]string{"1", "2"}, func(arg string) (*pb.Build, error) {
				if arg == "1" {
					return build, nil
				}
				return nil, fmt.Errorf("bad")
			})

			So(res, ShouldHaveLength, 2)

			So(res[0].arg, ShouldEqual, "1")
			So(res[0].err, ShouldBeNil)
			So(res[0].build, ShouldResembleProto, build)

			So(res[1].arg, ShouldEqual, "2")
			So(res[1].err, ShouldErrLike, "bad")
			So(res[1].build, ShouldBeNil)
		})
	})
}
