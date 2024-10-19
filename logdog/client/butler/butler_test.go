// Copyright 2015 The LUCI Authors.
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

package butler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bootstrap"
	"go.chromium.org/luci/logdog/client/butler/output"
	"go.chromium.org/luci/logdog/client/butler/output/null"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
	"go.chromium.org/luci/logdog/common/types"
)

type testOutput struct {
	sync.Mutex

	err      error
	maxSize  int
	streams  map[string][]*logpb.LogEntry
	terminal map[string]struct{}

	closed bool
}

func (to *testOutput) SendBundle(b *logpb.ButlerLogBundle) error {
	if to.err != nil {
		return to.err
	}

	to.Lock()
	defer to.Unlock()

	if to.streams == nil {
		to.streams = map[string][]*logpb.LogEntry{}
		to.terminal = map[string]struct{}{}
	}
	for _, be := range b.Entries {
		name := string(be.Desc.Name)

		to.streams[name] = append(to.streams[name], be.Logs...)
		if be.TerminalIndex >= 0 {
			to.terminal[name] = struct{}{}
		}
	}
	return nil
}

func (to *testOutput) MaxSendBundles() int { return 1 }

func (to *testOutput) MaxSize() int {
	return to.maxSize
}

func (to *testOutput) Stats() output.Stats {
	return &output.StatsBase{}
}

func (to *testOutput) URLConstructionEnv() bootstrap.Environment {
	return bootstrap.Environment{}
}

func (to *testOutput) Close() {
	if to.closed {
		panic("double close")
	}
	to.closed = true
}

func (to *testOutput) logs(name string) []*logpb.LogEntry {
	to.Lock()
	defer to.Unlock()

	return to.streams[name]
}

func (to *testOutput) isTerminal(name string) bool {
	to.Lock()
	defer to.Unlock()

	_, isTerminal := to.terminal[name]
	return isTerminal
}

func shouldHaveTextLogs(expected ...string) comparison.Func[[]*logpb.LogEntry] {
	const cmpName = "shouldHaveTextLogs"

	return func(actual []*logpb.LogEntry) *failure.Summary {
		var lines []string
		var prev string
		var nonTextIndexes []int
		for i, le := range actual {
			if le.GetText() == nil {
				nonTextIndexes = append(nonTextIndexes, i)
				continue
			}

			for _, l := range le.GetText().Lines {
				prev += string(l.Value)
				if l.Delimiter != "" {
					lines = append(lines, prev)
					prev = ""
				}
			}
		}
		if len(nonTextIndexes) > 0 {
			return comparison.NewSummaryBuilder(cmpName).
				Because("The following actual LogEntry's contained non-text data: %#v", nonTextIndexes).
				Summary
		}

		// Add any trailing (non-delimited) data.
		if prev != "" {
			lines = append(lines, prev)
		}

		ret := should.Match(expected)(lines)
		if ret != nil {
			ret.Comparison.Name = cmpName
		}
		return ret
	}
}

type testStreamData struct {
	data []byte
	err  error
}

type testStream struct {
	inC     chan *testStreamData
	closedC chan struct{}

	allowDoubleClose bool

	desc *logpb.LogStreamDescriptor
}

func (ts *testStream) data(d []byte, err error) {
	ts.inC <- &testStreamData{
		data: d,
		err:  err,
	}
	if err == io.EOF {
		// If EOF is hit, continue reporting EOF.
		close(ts.inC)
	}
}

func (ts *testStream) err(err error) {
	ts.data(nil, err)
}

func (ts *testStream) isClosed() bool {
	select {
	case <-ts.closedC:
		return true
	default:
		return false
	}
}

func (ts *testStream) Close() error {
	if ts.isClosed() {
		if ts.allowDoubleClose {
			return nil
		}
		panic("double close")
	}

	close(ts.closedC)
	return nil
}

func (ts *testStream) Read(b []byte) (int, error) {
	select {
	case <-ts.closedC:
		return 0, io.EOF

	case d, ok := <-ts.inC:
		// We have data on "inC", but we want closed status to trump this.
		if !ok || ts.isClosed() {
			return 0, io.EOF
		}
		return copy(b, d.data), d.err
	}
}

type testStreamServer struct {
	err     error
	onNext  func()
	streamC chan *testStream
}

func newTestStreamServer() *testStreamServer {
	return &testStreamServer{
		streamC: make(chan *testStream),
	}
}

func (tss *testStreamServer) Address() string { return "test" }

func (tss *testStreamServer) Listen() error { return tss.err }

func (tss *testStreamServer) Next() (io.ReadCloser, *logpb.LogStreamDescriptor) {
	if tss.onNext != nil {
		tss.onNext()
	}

	ts, ok := <-tss.streamC
	if !ok {
		return nil, nil
	}
	return ts, ts.desc
}

func (tss *testStreamServer) Close() {
	close(tss.streamC)
}

func (tss *testStreamServer) enqueue(ts *testStream) {
	tss.streamC <- ts
}

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run(`A Config instance`, t, func(t *ftt.Test) {
		to := testOutput{
			maxSize: 1024,
		}
		conf := Config{
			Output: &to,
		}

		t.Run(`Will validate.`, func(t *ftt.Test) {
			assert.Loosely(t, conf.Validate(), should.BeNil)
		})

		t.Run(`Will not validate with a nil Output.`, func(t *ftt.Test) {
			conf.Output = nil
			assert.Loosely(t, conf.Validate(), should.ErrLike("an Output must be supplied"))
		})
	})
}

// mkb is shorthand for "make Butler". It calls "new" and panics if there is
// an error.
func mkb(c context.Context, config Config) *Butler {
	b, err := New(c, config)
	if err != nil {
		panic(err)
	}
	return b
}

func TestButler(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing Butler instance`, t, func(t *ftt.Test) {
		c, _ := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		to := testOutput{
			maxSize: 1024,
		}

		conf := Config{
			Output:     &to,
			BufferLogs: false,
		}

		t.Run(`Will error if an invalid Config is passed.`, func(t *ftt.Test) {
			conf.Output = nil
			assert.Loosely(t, conf.Validate(), should.NotBeNil)

			_, err := New(c, conf)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run(`Will Run until Activated, then shut down.`, func(t *ftt.Test) {
			conf.BufferLogs = true // (Coverage)

			b := mkb(c, conf)
			b.Activate()
			assert.Loosely(t, b.Wait(), should.BeNil)
		})

		t.Run(`Will forward a Shutdown error.`, func(t *ftt.Test) {
			conf.BufferLogs = true // (Coverage)

			b := mkb(c, conf)
			b.shutdown(errors.New("shutdown error"))
			assert.Loosely(t, b.Wait(), should.ErrLike("shutdown error"))
		})

		t.Run(`Will retain the first error and ignore duplicate shutdowns.`, func(t *ftt.Test) {
			b := mkb(c, conf)
			b.shutdown(errors.New("first error"))
			b.shutdown(errors.New("second error"))
			assert.Loosely(t, b.Wait(), should.ErrLike("first error"))
		})

		t.Run(`Using a generic stream Properties`, func(t *ftt.Test) {
			newTestStream := func(setup func(d *logpb.LogStreamDescriptor)) *testStream {
				desc := &logpb.LogStreamDescriptor{
					Name:        "test",
					StreamType:  logpb.StreamType_TEXT,
					ContentType: string(types.ContentTypeText),
				}
				if setup != nil {
					setup(desc)
				}

				return &testStream{
					inC:     make(chan *testStreamData, 16),
					closedC: make(chan struct{}),
					desc:    desc,
				}
			}

			t.Run(`Will not add a stream with an invalid configuration.`, func(t *ftt.Test) {
				// No content type.
				s := newTestStream(func(d *logpb.LogStreamDescriptor) {
					d.ContentType = ""
				})
				b := mkb(c, conf)
				assert.Loosely(t, b.AddStream(s, s.desc), should.NotBeNil)
			})

			t.Run(`Will not add a stream with a duplicate stream name.`, func(t *ftt.Test) {
				b := mkb(c, conf)

				s0 := newTestStream(nil)
				assert.Loosely(t, b.AddStream(s0, s0.desc), should.BeNil)

				s1 := newTestStream(nil)
				assert.Loosely(t, b.AddStream(s1, s1.desc), should.ErrLike("duplicate registration"))
			})

			t.Run(`Can apply global tags.`, func(t *ftt.Test) {
				conf.GlobalTags = streamproto.TagMap{
					"foo": "bar",
					"baz": "qux",
				}
				desc := &logpb.LogStreamDescriptor{
					Name:        "stdout",
					ContentType: "test/data",
					Timestamp:   timestamppb.New(time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)),
				}

				closeStreams := make(chan *testStreamData)

				// newTestStream returns a stream that hangs in Read() until
				// 'closeStreams' is closed. We do this to ensure Bundler doesn't
				// "drain" and unregister the stream before GetStreamDescs() calls
				// below have a chance to notice it exists.
				newTestStream := func() io.ReadCloser {
					return &testStream{
						closedC: make(chan struct{}),
						inC:     closeStreams,
					}
				}

				b := mkb(c, conf)
				defer func() {
					close(closeStreams)
					b.Activate()
					b.Wait()
				}()

				t.Run(`Applies global tags, but allows the stream to override.`, func(t *ftt.Test) {
					desc.Tags = map[string]string{
						"baz": "override",
					}

					assert.Loosely(t, b.AddStream(newTestStream(), desc), should.BeNil)
					assert.Loosely(t, b.bundler.GetStreamDescs()["stdout"], should.Resemble(&logpb.LogStreamDescriptor{
						Name:        "stdout",
						ContentType: "test/data",
						Timestamp:   desc.Timestamp,
						Tags: map[string]string{
							"foo": "bar",
							"baz": "override",
						},
					}))
				})

				t.Run(`Will apply global tags if the stream has none (nil).`, func(t *ftt.Test) {
					assert.Loosely(t, b.AddStream(newTestStream(), desc), should.BeNil)
					assert.Loosely(t, b.bundler.GetStreamDescs()["stdout"], should.Resemble(&logpb.LogStreamDescriptor{
						Name:        "stdout",
						ContentType: "test/data",
						Timestamp:   desc.Timestamp,
						Tags: map[string]string{
							"foo": "bar",
							"baz": "qux",
						},
					}))
				})
			})

			t.Run(`Run with 256 streams, stream{0..256} will deplete and finish.`, func(t *ftt.Test) {
				b := mkb(c, conf)
				streams := make([]*testStream, 256)
				for i := range streams {
					streams[i] = newTestStream(func(d *logpb.LogStreamDescriptor) {
						d.Name = fmt.Sprintf("stream%d", i)
					})
				}

				for _, s := range streams {
					assert.Loosely(t, b.AddStream(s, s.desc), should.BeNil)
					s.data([]byte("stream data 0!\n"), nil)
					s.data([]byte("stream data 1!\n"), nil)
				}

				// Add data to the streams after shutdown.
				for _, s := range streams {
					s.data([]byte("stream data 2!\n"), io.EOF)
				}

				b.Activate()
				assert.Loosely(t, b.Wait(), should.BeNil)

				for _, s := range streams {
					name := string(s.desc.Name)

					assert.Loosely(t, to.logs(name), shouldHaveTextLogs("stream data 0!", "stream data 1!", "stream data 2!"))
					assert.Loosely(t, to.isTerminal(name), should.BeTrue)
				}
			})

			t.Run(`Shutdown with 256 in-progress streams, stream{0..256} will terminate if they emitted logs.`, func(t *ftt.Test) {
				b := mkb(c, conf)
				streams := make([]*testStream, 256)
				for i := range streams {
					streams[i] = newTestStream(func(d *logpb.LogStreamDescriptor) {
						d.Name = fmt.Sprintf("stream%d", i)
					})
				}

				for _, s := range streams {
					assert.Loosely(t, b.AddStream(s, s.desc), should.BeNil)
					s.data([]byte("stream data!\n"), nil)
				}

				b.shutdown(errors.New("test shutdown"))
				assert.Loosely(t, b.Wait(), should.ErrLike("test shutdown"))

				for _, s := range streams {
					if len(to.logs(s.desc.Name)) > 0 {
						assert.Loosely(t, to.isTerminal(string(s.desc.Name)), should.BeTrue)
					} else {
						assert.Loosely(t, to.isTerminal(string(s.desc.Name)), should.BeFalse)
					}
				}
			})

			t.Run(`Using ten test stream servers`, func(t *ftt.Test) {
				servers := make([]*testStreamServer, 10)
				for i := range servers {
					servers[i] = newTestStreamServer()
				}
				streams := []*testStream(nil)

				t.Run(`Can register both before Run and will retain streams.`, func(t *ftt.Test) {
					b := mkb(c, conf)
					for i, tss := range servers {
						b.AddStreamServer(tss)

						s := newTestStream(func(d *logpb.LogStreamDescriptor) {
							d.Name = fmt.Sprintf("stream%d", i)
						})
						streams = append(streams, s)
						s.data([]byte("test data"), io.EOF)
						tss.enqueue(s)
					}

					b.Activate()
					assert.Loosely(t, b.Wait(), should.BeNil)

					for _, s := range streams {
						assert.Loosely(t, to.logs(s.desc.Name), shouldHaveTextLogs("test data"))
						assert.Loosely(t, to.isTerminal(s.desc.Name), should.BeTrue)
					}
				})

				t.Run(`Can register both during Run and will retain streams.`, func(t *ftt.Test) {
					b := mkb(c, conf)
					for i, tss := range servers {
						b.AddStreamServer(tss)

						s := newTestStream(func(d *logpb.LogStreamDescriptor) {
							d.Name = fmt.Sprintf("stream%d", i)
						})
						streams = append(streams, s)
						s.data([]byte("test data"), io.EOF)
						tss.enqueue(s)
					}

					b.Activate()
					assert.Loosely(t, b.Wait(), should.BeNil)

					for _, s := range streams {
						assert.Loosely(t, to.logs(s.desc.Name), shouldHaveTextLogs("test data"))
						assert.Loosely(t, to.isTerminal(s.desc.Name), should.BeTrue)
					}
				})
			})

			t.Run(`Will ignore stream registration errors, allowing re-registration.`, func(t *ftt.Test) {
				tss := newTestStreamServer()

				// Generate an invalid stream for "tss" to register.
				sGood := newTestStream(nil)
				sGood.data([]byte("good test data"), io.EOF)

				sBad := newTestStream(func(d *logpb.LogStreamDescriptor) {
					d.ContentType = ""
				})
				sBad.data([]byte("bad test data"), io.EOF)

				b := mkb(c, conf)
				b.AddStreamServer(tss)
				tss.enqueue(sBad)
				tss.enqueue(sGood)
				b.Activate()
				assert.Loosely(t, b.Wait(), should.BeNil)

				assert.Loosely(t, sBad.isClosed(), should.BeTrue)
				assert.Loosely(t, sGood.isClosed(), should.BeTrue)
				assert.Loosely(t, to.logs("test"), shouldHaveTextLogs("good test data"))
				assert.Loosely(t, to.isTerminal("test"), should.BeTrue)
			})
		})

		t.Run(`Will terminate if the stream server panics.`, func(t *ftt.Test) {
			tss := newTestStreamServer()
			tss.onNext = func() {
				panic("test panic")
			}

			b := mkb(c, conf)
			b.AddStreamServer(tss)
			assert.Loosely(t, b.Wait(), should.ErrLike("test panic"))
		})

		t.Run(`Can wait for a subset of streams to complete.`, func(t *ftt.Test) {
			c = logging.SetLevel(gologger.StdConfig.Use(c), logging.Debug)

			b := mkb(c, conf)
			defer func() {
				b.Activate()
				b.Wait()
			}()

			newTestStream := func(setup func(d *logpb.LogStreamDescriptor)) *testStream {
				desc := &logpb.LogStreamDescriptor{
					Name:        "test",
					StreamType:  logpb.StreamType_TEXT,
					ContentType: string(types.ContentTypeText),
				}
				if setup != nil {
					setup(desc)
				}

				return &testStream{
					inC:              make(chan *testStreamData, 16),
					closedC:          make(chan struct{}),
					desc:             desc,
					allowDoubleClose: true,
				}
			}

			deep := newTestStream(func(d *logpb.LogStreamDescriptor) { d.Name = "ns/deep/s" })
			defer deep.Close()

			ns := newTestStream(func(d *logpb.LogStreamDescriptor) { d.Name = "ns/s" })
			defer ns.Close()

			s := newTestStream(func(d *logpb.LogStreamDescriptor) { d.Name = "s" })
			defer s.Close()

			b.AddStream(deep, deep.desc)
			b.AddStream(ns, ns.desc)
			b.AddStream(s, s.desc)

			wait := func(t testing.TB, ns ...types.StreamName) <-chan struct{} {
				ch := make(chan struct{})
				ret := sync.WaitGroup{}
				ret.Add(len(ns))
				go func() {
					ret.Wait()
					close(ch)
				}()
				for _, singleNS := range ns {
					singleNS := singleNS
					go func() {
						defer ret.Done()
						assert.Loosely(t, b.DrainNamespace(c, singleNS), should.BeEmpty)
					}()
				}
				return ch
			}

			check := func(toWait <-chan struct{}, toClose ...*testStream) {
				select {
				case <-time.After(time.Millisecond):
				case <-toWait:
					panic("we should time out here")
				}

				for _, c := range toClose {
					assert.Loosely(t, c.Close(), should.BeNil)
				}
				<-toWait
			}

			t.Run(`waiting for already-empty namespace works`, func(t *ftt.Test) {
				assert.Loosely(t, b.DrainNamespace(c, "other"), should.BeNil)
			})

			t.Run(`can wait at a deep level`, func(t *ftt.Test) {
				check(wait(t, "ns/deep"), deep)

				t.Run(`and we now cannot open new streams under that namespace`, func(t *ftt.Test) {
					err := b.AddStream(nil, &logpb.LogStreamDescriptor{
						Name:        "ns/deep/other",
						ContentType: "wat",
					})
					assert.Loosely(t, err, should.ErrLike(`namespace "ns/deep/": already closed`))
				})

				t.Run(`can still open new adjacent streams`, func(t *ftt.Test) {
					cool := newTestStream(func(d *logpb.LogStreamDescriptor) { d.Name = "ns/side/other" })
					defer cool.Close()
					assert.Loosely(t, b.AddStream(cool, cool.desc), should.BeNil)
				})

				t.Run(`then we can also wait at higher levels`, func(t *ftt.Test) {
					check(wait(t, "ns"), ns)
				})
			})

			t.Run(`can wait at multiple levels`, func(t *ftt.Test) {
				check(wait(t, "ns", "ns/deep"), ns, deep)
			})

			t.Run(`can cancel the wait and see leftovers`, func(t *ftt.Test) {
				cctx, cancel := context.WithCancel(c)
				cancel()
				assert.Loosely(t, b.DrainNamespace(cctx, "ns"), should.Resemble([]types.StreamName{
					"ns/deep/s",
					"ns/s",
				}))
			})

			t.Run(`can cancel entire namespace`, func(t *ftt.Test) {
				// TODO(iannucci): We should use this in Wait() for a more orderly
				// shutdown.
				check(wait(t, ""), ns, deep, s)
			})
		})
	})
}

// Command to run: `go test -cpu 1,4,8  -benchmem -benchtime 5s -bench .`
func BenchmarkButler(b *testing.B) {
	numOfStreams := []int{10, 100, 500}
	testText := []string{
		"This is line one\n",
		"This is another two\n",
		"This is final line",
	}

	for _, n := range numOfStreams {
		b.Run(fmt.Sprintf("%d-streams", n), func(b *testing.B) {
			for i := 0; i <= b.N; i++ {
				if err := runButlerBenchmark(testText, n); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// runButlerBenchmark creates a new butler instance and given number of
// streams, then write the supplied text to each stream and wait for
// the butler instance to complete.
func runButlerBenchmark(testText []string, numOfStream int) error {
	conf := Config{
		Output:     &null.Output{},
		BufferLogs: false,
	}
	tb := mkb(context.Background(), conf)
	tss := newTestStreamServer()
	tb.AddStreamServer(tss)

	testStreams := make([]*testStream, numOfStream)

	for i := range testStreams {
		testStreams[i] = &testStream{
			inC:     make(chan *testStreamData, 16),
			closedC: make(chan struct{}),
			desc: &logpb.LogStreamDescriptor{
				Name:        fmt.Sprintf("stream-%d", i),
				StreamType:  logpb.StreamType_TEXT,
				ContentType: string(types.ContentTypeText),
			},
		}
		tss.enqueue(testStreams[i])
	}

	for _, ts := range testStreams {
		go func(s *testStream) {
			for _, line := range testText {
				s.data([]byte(line), nil)
			}
			s.err(io.EOF)
		}(ts)
	}

	tb.Activate()
	if err := tb.Wait(); err != nil {
		return err
	}
	return nil
}
