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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestPrintAndDone(t *testing.T) {
	t.Parallel()

	ftt.Run("printAndDone", t, func(c *ftt.Test) {
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

		c.Run("actual args", func(c *ftt.Test) {
			var m sync.Mutex
			actualArgs := stringset.New(3)
			call([]string{"1", "2"}, func(arg string) (*pb.Build, error) {
				m.Lock()
				defer m.Unlock()
				actualArgs.Add(arg)
				return &pb.Build{SummaryMarkdown: arg}, nil
			})
			assert.Loosely(c, actualArgs, should.Match(stringset.NewFromSlice("1", "2")))
		})

		c.Run("one build", func(c *ftt.Test) {
			build := &pb.Build{SummaryMarkdown: "1"}
			res := call([]string{"1"}, func(arg string) (*pb.Build, error) {
				return build, nil
			})
			assert.Loosely(c, res[0].arg, should.Equal("1"))
			assert.Loosely(c, res[0].err, should.BeNil)
			assert.Loosely(c, res[0].build, should.Match(build))
		})

		c.Run("two builds", func(c *ftt.Test) {
			build := &pb.Build{SummaryMarkdown: "1"}
			res := call([]string{"1", "2"}, func(arg string) (*pb.Build, error) {
				if arg == "1" {
					return build, nil
				}
				return nil, fmt.Errorf("bad")
			})

			assert.Loosely(c, res, should.HaveLength(2))

			assert.Loosely(c, res[0].arg, should.Equal("1"))
			assert.Loosely(c, res[0].err, should.BeNil)
			assert.Loosely(c, res[0].build, should.Match(build))

			assert.Loosely(c, res[1].arg, should.Equal("2"))
			assert.Loosely(c, res[1].err, should.ErrLike("bad"))
			assert.Loosely(c, res[1].build, should.BeNil)
		})
	})
}
