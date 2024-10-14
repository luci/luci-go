// Copyright 2016 The LUCI Authors.
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

package coordinator

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/common/types"
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

func (c *testCoordinator) RegisterStream(ctx context.Context, s *LogStreamState, desc []byte) (
	*LogStreamState, error) {
	if err := c.incCalls(); err != nil {
		return nil, err
	}

	// Set the ProtoVersion to differentiate the output State from the input.
	rs := *s
	rs.ProtoVersion = "remote"
	return &rs, nil
}

func (c *testCoordinator) TerminateStream(ctx context.Context, tr *TerminateRequest) error {
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

	ftt.Run(`Using a test configuration`, t, func(t *ftt.Test) {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		tcc := testCoordinator{}

		st := LogStreamState{
			Project:       "test-project",
			Path:          "test-stream-path",
			ID:            "hash12345",
			TerminalIndex: -1,
		}

		tr := TerminateRequest{
			Project:       st.Project,
			Path:          st.Path,
			ID:            st.ID,
			TerminalIndex: 1337,
			Secret:        st.Secret,
		}

		// Note: In all of these tests, we check if "proto" field (ProtoVersion)
		// is "remote". We use ProtoVersion as a channel between our fake remote
		// service. When our fake remote service returns a LogStreamState, it sets
		// "remote" to true to differentiate it from the local pushed state.
		//
		// If a LogStreamState has "remote" set to true, that implies that it was
		// sent by the fake testing service rather than the local test.
		t.Run(`A streamStateCache`, func(t *ftt.Test) {
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

			t.Run(`Can register a stream`, func(t *ftt.Test) {
				s, err := ssc.RegisterStream(c, &st, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s.ProtoVersion, should.Equal("remote"))
				assert.Loosely(t, tcc.calls, should.Equal(1))

				t.Run(`Will not re-register the same stream.`, func(t *ftt.Test) {
					st.ProtoVersion = ""

					s, err := ssc.RegisterStream(c, &st, nil)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, s.ProtoVersion, should.Equal("remote"))
					assert.Loosely(t, tcc.calls, should.Equal(1))
				})

				t.Run(`When the registration expires`, func(t *ftt.Test) {
					st.ProtoVersion = ""
					tc.Add(time.Second)

					t.Run(`Will re-register the stream.`, func(t *ftt.Test) {
						s, err := ssc.RegisterStream(c, &st, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, s.ProtoVersion, should.Equal("remote"))
						assert.Loosely(t, tcc.calls, should.Equal(2))
					})
				})

				t.Run(`Can terminate a registered stream`, func(t *ftt.Test) {
					assert.Loosely(t, ssc.TerminateStream(c, &tr), should.BeNil)
					assert.Loosely(t, tcc.calls, should.Equal(2)) // +1 call

					t.Run(`Registering the stream will include the terminal index.`, func(t *ftt.Test) {
						// Fill it in with junk to make sure we are getting cached.
						st.TerminalIndex = 123
						st.ProtoVersion = ""

						s, err := ssc.RegisterStream(c, &st, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, s.ProtoVersion, should.Equal("remote"))
						assert.Loosely(t, s.TerminalIndex, should.Equal(1337))
						assert.Loosely(t, tcc.calls, should.Equal(2)) // No additional calls.
					})
				})
			})

			t.Run(`Can register a stream with a terminal index`, func(t *ftt.Test) {
				st.TerminalIndex = 1337

				s, err := ssc.RegisterStream(c, &st, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, s.ProtoVersion, should.Equal("remote"))
				assert.Loosely(t, tcc.calls, should.Equal(1))

				t.Run(`A subsequent call to TerminateStream will be ignored, since we have remote terminal confirmation.`, func(t *ftt.Test) {
					tr.TerminalIndex = 12345

					assert.Loosely(t, ssc.TerminateStream(c, &tr), should.BeNil)
					assert.Loosely(t, tcc.calls, should.Equal(1)) // (No additional calls)

					t.Run(`A register stream call will return the confirmed terminal index.`, func(t *ftt.Test) {
						st.TerminalIndex = 0

						s, err := ssc.RegisterStream(c, &st, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, s.ProtoVersion, should.Equal("remote"))
						assert.Loosely(t, tcc.calls, should.Equal(1)) // (No additional calls)
						assert.Loosely(t, s.TerminalIndex, should.Equal(1337))
					})
				})

				t.Run(`A subsqeuent register stream call will return the confirmed terminal index.`, func(t *ftt.Test) {
					st.TerminalIndex = 0

					s, err := ssc.RegisterStream(c, &st, nil)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, s.ProtoVersion, should.Equal("remote"))
					assert.Loosely(t, tcc.calls, should.Equal(1)) // (No additional calls)
					assert.Loosely(t, s.TerminalIndex, should.Equal(1337))
				})
			})

			t.Run(`When multiple goroutines register the same stream, it gets registered once.`, func(t *ftt.Test) {
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

				assert.Loosely(t, errors.SingleError(errs), should.BeNil)
				assert.Loosely(t, tcc.calls, should.Equal(1))
			})

			t.Run(`Multiple registrations for the same stream will result in two requests if the first expires.`, func(t *ftt.Test) {
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

				assert.Loosely(t, r1.ProtoVersion, should.Equal("remote"))
				assert.Loosely(t, r2.ProtoVersion, should.Equal("remote"))
				assert.Loosely(t, tcc.calls, should.Equal(2))
			})

			t.Run(`RegisterStream`, func(t *ftt.Test) {
				t.Run(`A transient registration error will result in a RegisterStream error.`, func(t *ftt.Test) {
					tcc.errC = make(chan error, 1)
					tcc.errC <- errors.New("test error", transient.Tag)

					_, err := ssc.RegisterStream(c, &st, nil)
					assert.Loosely(t, err, should.NotBeNil)
					assert.Loosely(t, tcc.calls, should.Equal(1))

					t.Run(`A second request will call through, try again, and succeed.`, func(t *ftt.Test) {
						tcc.errC = nil

						_, err := ssc.RegisterStream(c, &st, nil)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, tcc.calls, should.Equal(2))
					})
				})

				t.Run(`A non-transient registration error will result in a RegisterStream error.`, func(t *ftt.Test) {
					tcc.errC = make(chan error, 1)
					tcc.errC <- errors.New("test error")

					_, err := ssc.RegisterStream(c, &st, nil)
					assert.Loosely(t, err, should.NotBeNil)
					assert.Loosely(t, tcc.calls, should.Equal(1))

					t.Run(`A second request will return the cached error.`, func(t *ftt.Test) {
						tcc.errC = nil

						_, err := ssc.RegisterStream(c, &st, nil)
						assert.Loosely(t, err, should.NotBeNil)
						assert.Loosely(t, tcc.calls, should.Equal(1))
					})
				})
			})

			t.Run(`TerminateStream`, func(t *ftt.Test) {
				tr := TerminateRequest{
					Project:       st.Project,
					ID:            st.ID,
					TerminalIndex: 1337,
				}

				t.Run(`The termination endpoint returns a transient error, it will propagate.`, func(t *ftt.Test) {
					tcc.errC = make(chan error, 1)
					tcc.errC <- errors.New("test error", transient.Tag)

					err := ssc.TerminateStream(c, &tr)
					assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
					assert.Loosely(t, tcc.calls, should.Equal(1))

					t.Run(`A second attempt will call through, try again, and succeed.`, func(t *ftt.Test) {
						tcc.errC = nil

						err := ssc.TerminateStream(c, &tr)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, tcc.calls, should.Equal(2))
					})
				})

				t.Run(`When the termination endpoint returns a non-transient error, it will propagate.`, func(t *ftt.Test) {
					tcc.errC = make(chan error, 1)
					tcc.errC <- errors.New("test error")

					err := ssc.TerminateStream(c, &tr)
					assert.Loosely(t, err, should.NotBeNil)
					assert.Loosely(t, tcc.calls, should.Equal(1))

					t.Run(`A second request will return the cached error.`, func(t *ftt.Test) {
						tcc.errC = nil

						err := ssc.TerminateStream(c, &tr)
						assert.Loosely(t, err, should.NotBeNil)
						assert.Loosely(t, tcc.calls, should.Equal(1))
					})
				})
			})

			t.Run(`Different projects with the same stream name will not conflict.`, func(t *ftt.Test) {
				var projects = []string{"", "foo", "bar"}

				for i, p := range projects {
					st.Project = p
					s, err := ssc.RegisterStream(c, &st, nil)
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, ssc.TerminateStream(c, &TerminateRequest{
						Project:       s.Project,
						Path:          s.Path,
						ID:            s.ID,
						TerminalIndex: types.MessageIndex(i),
					}), should.BeNil)
				}
				assert.Loosely(t, tcc.calls, should.Equal(len(projects)*2))

				for i, p := range projects {
					st.Project = p
					st.TerminalIndex = -1

					s, err := ssc.RegisterStream(c, &st, nil)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, s.TerminalIndex, should.Equal(types.MessageIndex(i)))
				}
				assert.Loosely(t, tcc.calls, should.Equal(len(projects)*2))
			})
		})

		t.Run(`A streamStateCache can register multiple streams at once.`, func(t *ftt.Test) {
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
				st.Path = types.StreamPath(fmt.Sprintf("ID:%d", i))

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
			assert.Loosely(t, errors.SingleError(errs), should.BeNil)

			remotes := 0
			for i := 0; i < count; i++ {
				if state[i].ProtoVersion == "remote" {
					remotes++
				}
			}
			assert.Loosely(t, remotes, should.Equal(count))
		})
	})
}
