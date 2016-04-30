// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

// testCoordinator is an implementation of Coordinator that can be used for
// testing.
type testCoordinator struct {
	// calls is the number of calls made to the interface's methods.
	calls int32
	// callC, if not nil, will have a token pushed to it when a call is made.
	callC chan struct{}
	// errC is a channel that error status will be read from if not nil.
	errC chan error
}

func (c *testCoordinator) RegisterStream(ctx context.Context, s *LogStreamState, d *logpb.LogStreamDescriptor) (
	*LogStreamState, error) {
	if err := c.incCalls(); err != nil {
		return nil, err
	}

	// Set the ProtoVersion to differentiate the output State from the input.
	rs := *s
	rs.ProtoVersion = "remote"
	return &rs, nil
}

func (c *testCoordinator) TerminateStream(ctx context.Context, st *LogStreamState) error {
	if err := c.incCalls(); err != nil {
		return err
	}
	return nil
}

// incCalls is an entry point for client goroutines. It offers the opportunity
// to track call count as well as trap executing goroutines within client calls.
//
// This must not be called while the lock is held, else it could lead to
// deadlock if multiple goroutines are trapped.
func (c *testCoordinator) incCalls() error {
	if c.callC != nil {
		c.callC <- struct{}{}
	}

	atomic.AddInt32(&c.calls, 1)

	if c.errC != nil {
		return <-c.errC
	}
	return nil
}

func TestStreamStateCache(t *testing.T) {
	t.Parallel()

	Convey(`Using a test configuration`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		tcc := testCoordinator{}

		st := LogStreamState{
			Project:       "test-project",
			Path:          "foo/+/bar",
			TerminalIndex: -1,
		}

		// Note: In all of these tests, we check if "proto" field (ProtoVersion)
		// is "remote". We use ProtoVersion as a channel between our fake remote
		// service. When our fake remote service returns a LogStreamState, it sets
		// "remote" to true to differentiate it from the local pushed state.
		//
		// If a LogStreamState has "remote" set to true, that implies that it was
		// sent by the fake testing service rather than the local test.
		Convey(`A streamStateCache`, func() {
			ssc := NewCache(&tcc, 4, 1*time.Second)

			resultC := make(chan *LogStreamState)
			req := func(st *LogStreamState) {
				var res *LogStreamState
				defer func() {
					resultC <- res
				}()

				st, err := ssc.RegisterStream(c, st, nil)
				if err == nil {
					res = st
				}
			}

			Convey(`Can register a stream`, func() {
				s, err := ssc.RegisterStream(c, &st, nil)
				So(err, ShouldBeNil)
				So(s.ProtoVersion, ShouldEqual, "remote")
				So(tcc.calls, ShouldEqual, 1)

				Convey(`Will not re-register the same stream.`, func() {
					st.ProtoVersion = ""

					s, err := ssc.RegisterStream(c, &st, nil)
					So(err, ShouldBeNil)
					So(s.ProtoVersion, ShouldEqual, "remote")
					So(tcc.calls, ShouldEqual, 1)
				})

				Convey(`When the registration expires`, func() {
					st.ProtoVersion = ""
					tc.Add(time.Second)

					Convey(`Will re-register the stream.`, func() {
						s, err := ssc.RegisterStream(c, &st, nil)
						So(err, ShouldBeNil)
						So(s.ProtoVersion, ShouldEqual, "remote")
						So(tcc.calls, ShouldEqual, 2)
					})
				})

				Convey(`Can terminate a registered stream`, func() {
					s.TerminalIndex = 1337
					So(ssc.TerminateStream(c, s), ShouldBeNil)
					So(tcc.calls, ShouldEqual, 2) // +1 call

					Convey(`Registering the stream will include the terminal index.`, func() {
						// Fill it in with junk to make sure we are getting cached.
						st.TerminalIndex = 123
						st.ProtoVersion = ""

						s, err := ssc.RegisterStream(c, &st, nil)
						So(err, ShouldBeNil)
						So(s.ProtoVersion, ShouldEqual, "remote")
						So(s.TerminalIndex, ShouldEqual, 1337)
						So(tcc.calls, ShouldEqual, 2) // No additional calls.
					})
				})
			})

			Convey(`When the terminal index is set before the fetch finishes, it will be returned.`, func() {
				tcc.callC = make(chan struct{})
				tcc.errC = make(chan error)

				go req(&st)

				// Wait for our request to block on RegisterStream.
				<-tcc.callC

				// Set the terminal index. We will use a minimal LogStreamState. We know
				// that this will happen after the streamStateCacheEntry is registered
				// because both block on the LRU cache's Mutate, which is atomic, and
				// RegisterStream must have added the streamStateCacheEntry in order for
				// the lock to be available for TerminateStream to proceed.
				terminalErrC := make(chan error)
				go func() {
					terminalErrC <- ssc.TerminateStream(c, &LogStreamState{
						Project:       st.Project,
						Path:          st.Path,
						TerminalIndex: 1337,
					})
				}()

				// Let both requests succeed.
				<-tcc.callC
				tcc.errC <- nil
				tcc.errC <- nil

				// Read the LogStreamState from our RegisterStream request.
				s := <-resultC
				So(s, ShouldNotBeNil)
				So(s.TerminalIndex, ShouldEqual, 1337)
			})

			Convey(`When multiple goroutines register the same stream, it gets registered once.`, func() {
				tcc.callC = make(chan struct{})
				tcc.errC = make(chan error)

				errs := make(errors.MultiError, 256)
				for i := 0; i < len(errs); i++ {
					go req(&st)
				}

				<-tcc.callC
				tcc.errC <- nil
				for i := 0; i < len(errs); i++ {
					<-resultC
				}

				So(errors.SingleError(errs), ShouldBeNil)
				So(tcc.calls, ShouldEqual, 1)
			})

			Convey(`Multiple registrations for the same stream will result in two requests if the first expires.`, func() {
				tcc.callC = make(chan struct{})
				tcc.errC = make(chan error)

				// First request.
				go req(&st)

				// Wait for the request to happen, then advance time past the request's
				// expiration.
				<-tcc.callC
				tc.Add(time.Second)

				// Second request.
				go req(&st)

				// Release both calls and reap the results.
				<-tcc.callC
				tcc.errC <- nil
				tcc.errC <- nil

				r1 := <-resultC
				r2 := <-resultC

				So(r1.ProtoVersion, ShouldEqual, "remote")
				So(r2.ProtoVersion, ShouldEqual, "remote")
				So(tcc.calls, ShouldEqual, 2)
			})

			Convey(`A registration error will result in a RegisterStream error.`, func() {
				tcc.errC = make(chan error, 1)
				tcc.errC <- errors.New("test error")

				_, err := ssc.RegisterStream(c, &st, nil)
				So(err, ShouldNotBeNil)
				So(tcc.calls, ShouldEqual, 1)

				Convey(`A second registration without error will make a new request.`, func() {
					tcc.errC = nil

					_, err := ssc.RegisterStream(c, &st, nil)
					So(err, ShouldBeNil)
					So(tcc.calls, ShouldEqual, 2)
				})
			})

			Convey(`Different projects with the sme stream name will not conflict.`, func() {
				var projects = []config.ProjectName{"", "foo", "bar"}

				for i, p := range projects {
					st.Project = p
					s, err := ssc.RegisterStream(c, &st, nil)
					So(err, ShouldBeNil)

					s.TerminalIndex = types.MessageIndex(i)
					So(ssc.TerminateStream(c, s), ShouldBeNil)
				}
				So(tcc.calls, ShouldEqual, len(projects)*2)

				for i, p := range projects {
					st.Project = p
					st.TerminalIndex = -1

					s, err := ssc.RegisterStream(c, &st, nil)
					So(err, ShouldBeNil)
					So(s.TerminalIndex, ShouldEqual, types.MessageIndex(i))
				}
				So(tcc.calls, ShouldEqual, len(projects)*2)
			})
		})

		Convey(`A streamStateCache can register multiple streams at once.`, func() {
			ssc := NewCache(&tcc, 0, 0)
			tcc.callC = make(chan struct{})
			tcc.errC = make(chan error)

			count := 2048
			wg := sync.WaitGroup{}
			errs := make(errors.MultiError, count)
			state := make([]*LogStreamState, count)
			wg.Add(count)
			for i := 0; i < count; i++ {
				st := st
				st.Path = types.StreamPath(fmt.Sprintf("foo/+/bar%d", i))

				go func(i int) {
					defer wg.Done()
					state[i], errs[i] = ssc.RegisterStream(c, &st, nil)
				}(i)
			}

			// Wait for all of them to simultaneously call.
			for i := 0; i < count; i++ {
				<-tcc.callC
			}

			// They're all blocked on errC; allow them to continue.
			for i := 0; i < count; i++ {
				tcc.errC <- nil
			}

			// Wait for them to finish.
			wg.Wait()

			// Confirm that all registered successfully.
			So(errors.SingleError(errs), ShouldBeNil)

			remotes := 0
			for i := 0; i < count; i++ {
				if state[i].ProtoVersion == "remote" {
					remotes++
				}
			}
			So(remotes, ShouldEqual, count)
		})
	})
}
