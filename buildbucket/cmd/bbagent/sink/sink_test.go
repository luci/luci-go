// Copyright 2020 The LUCI Authors.
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

package sink

import (
	"context"
	"testing"
	"time"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"google.golang.org/protobuf/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuildSink(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	Convey(`BuildSink`, t, func() {
		outBuilds := []*bbpb.Build{}
		var outError error
		outfn := func(build *bbpb.Build) error {
			outBuilds = append(outBuilds, proto.Clone(build).(*bbpb.Build))
			return outError
		}
		fatalTag := errors.BoolTag{Key: errors.NewTagKey("fatal out error")}
		bs, err := NewBuildSink(ctx, outfn, func(err error) bool {
			return fatalTag.In(err)
		})
		So(err, ShouldBeNil)

		closeAndVerify := func() {
			bs.Close()
			mustHappen("closeC is not unblocked after `Close()` returns", func() {
				<-bs.closeC
			})
			var fatal error
			mustHappen("fatalC is not unblocked after `Close()` returns", func() {
				fatal = <-bs.fatalC
			})
			So(fatal, ShouldBeNil)
		}

		Convey(`Success`, func() {
			build := &bbpb.Build{Id: 1, Status: bbpb.Status_STARTED}
			bs.Sink(build)
			closeAndVerify()
			So(outBuilds, ShouldResembleProto, []*bbpb.Build{build})
		})

		Convey(`Non-fatal error`, func() {
			outError = errors.Reason("non-fatal error").Err()
			build := &bbpb.Build{Id: 1, Status: bbpb.Status_STARTED}
			bs.Sink(build)
			closeAndVerify()
			So(outBuilds, ShouldResembleProto, []*bbpb.Build{build})
		})

		Convey(`Fatal error`, func() {
			outError = errors.Reason("fatal error").Tag(fatalTag).Err()
			build := &bbpb.Build{Id: 1, Status: bbpb.Status_STARTED}
			bs.Sink(build)
			var fatal error
			mustHappen("fatalC doesn't receive fatal error", func() {
				fatal = <-bs.fatalC
			})
			So(fatal, ShouldErrLike, outError)

			mustHappen("outC is not closed and drained after fatal error occurs", func() {
				<-bs.outC.DrainC
			})
			// This build update won't be sinked due to the fatal.
			bs.Sink(&bbpb.Build{Id: 1, Status: bbpb.Status_FAILURE})
			closeAndVerify()
			So(outBuilds, ShouldResembleProto, []*bbpb.Build{build})
		})

		Convey(`Respect ctx.Done()`, func() {
			cancel()
			bs.Sink(&bbpb.Build{Id: 1, Status: bbpb.Status_SUCCESS})
			closeAndVerify()
			So(outBuilds, ShouldBeEmpty)
		})
	})
}

func mustHappen(desc string, blockingOperation func()) {
	c := make(chan struct{})
	go func() {
		blockingOperation()
		close(c)
	}()
	select {
	case <-c:
	case <-time.After(10 * time.Second):
		panic(desc)
	}
}
