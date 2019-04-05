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
	"bytes"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPrintAndDone(t *testing.T) {
	t.Parallel()

	Convey("printAndDone", t, func(c C) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		nowFn := func() time.Time { return testclock.TestRecentTimeUTC }
		stdoutPrinter := newPrinter(stdout, true, nowFn)
		stderrPrinter := newPrinter(stderr, true, nowFn)

		call := func(args []string, fn func(string) (*pb.Build, error)) int {
			run := &printRun{id: true}
			return run.printAndDone(stdoutPrinter, stderrPrinter, args, fn)
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

		Convey("perfect", func() {
			exitCode := call([]string{"1"}, func(arg string) (*pb.Build, error) {
				return &pb.Build{SummaryMarkdown: arg}, nil
			})
			So(exitCode, ShouldEqual, 0)
		})

		Convey("imperfect", func() {
			exitCode := call([]string{"1", "2"}, func(arg string) (*pb.Build, error) {
				if arg == "2" {
					return nil, fmt.Errorf("bad")
				}
				return &pb.Build{SummaryMarkdown: arg}, nil
			})
			So(exitCode, ShouldEqual, 1)
		})

		Convey("printed", func() {
			call([]string{"1", "2"}, func(arg string) (*pb.Build, error) {
				id, err := strconv.ParseInt(arg, 10, 64)
				c.So(err, ShouldBeNil)
				return &pb.Build{Id: id}, nil
			})
			So(stdout.String(), ShouldEqual, "1\n2\n")
		})
	})
}
