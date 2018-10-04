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

package localsrv

import (
	"context"
	"net"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func noopServe(context.Context, net.Listener, *sync.WaitGroup) error {
	return nil
}

func TestServerLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Double Start", t, func() {
		s := Server{}
		defer s.Stop(ctx)
		_, err := s.Start(ctx, "test", 0, noopServe)
		So(err, ShouldBeNil)
		_, err = s.Start(ctx, "test", 0, noopServe)
		So(err, ShouldErrLike, "already initialized")
	})

	Convey("Start after Stop", t, func() {
		s := Server{}
		_, err := s.Start(ctx, "test", 0, noopServe)
		So(err, ShouldBeNil)
		So(s.Stop(ctx), ShouldBeNil)
		_, err = s.Start(ctx, "test", 0, noopServe)
		So(err, ShouldErrLike, "already initialized")
	})

	Convey("Stop works", t, func() {
		serving := make(chan struct{})
		s := Server{}
		_, err := s.Start(ctx, "test", 0, func(c context.Context, l net.Listener, wg *sync.WaitGroup) error {
			close(serving)
			<-c.Done() // the context is canceled by Stop() call below
			return nil
		})
		So(err, ShouldBeNil)

		<-serving // wait until really started

		// Stop it.
		So(s.Stop(ctx), ShouldBeNil)
		// Doing it second time is ok too.
		So(s.Stop(ctx), ShouldBeNil)
	})
}
