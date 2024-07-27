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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"net"
	"sync"
	"testing"
)

func noopServe(context.Context, net.Listener, *sync.WaitGroup) error {
	return nil
}

func TestServerLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Double Start", t, func(t *ftt.Test) {
		s := Server{}
		defer s.Stop(ctx)
		_, err := s.Start(ctx, "test", 0, noopServe)
		assert.Loosely(t, err, should.BeNil)
		_, err = s.Start(ctx, "test", 0, noopServe)
		assert.Loosely(t, err, should.ErrLike("already initialized"))
	})

	ftt.Run("Start after Stop", t, func(t *ftt.Test) {
		s := Server{}
		_, err := s.Start(ctx, "test", 0, noopServe)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s.Stop(ctx), should.BeNil)
		_, err = s.Start(ctx, "test", 0, noopServe)
		assert.Loosely(t, err, should.ErrLike("already initialized"))
	})

	ftt.Run("Stop works", t, func(t *ftt.Test) {
		serving := make(chan struct{})
		s := Server{}
		_, err := s.Start(ctx, "test", 0, func(c context.Context, l net.Listener, wg *sync.WaitGroup) error {
			close(serving)
			<-c.Done() // the context is canceled by Stop() call below
			return nil
		})
		assert.Loosely(t, err, should.BeNil)

		<-serving // wait until really started

		// Stop it.
		assert.Loosely(t, s.Stop(ctx), should.BeNil)
		// Doing it second time is ok too.
		assert.Loosely(t, s.Stop(ctx), should.BeNil)
	})
}
