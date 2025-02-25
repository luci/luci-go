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

package logging

import (
	"fmt"
	"testing"

	"google.golang.org/grpc/grpclog"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// TestGRPCLogger assumes that it has exclusive ownership of the grpclog
// package. Each test MUST execute in series, as they install a global logger
// into the grpclog package.
func TestGRPCLogger(t *testing.T) {
	ftt.Run(`Testing gRPC logger adapter`, t, func(t *ftt.Test) {
		var base memlogger.MemLogger

		// Override "fatalExit" so our Fatal* tests don't actually exit.
		exitCalls := 0
		fatalExit = func() {
			exitCalls++
		}

		for _, l := range []logging.Level{
			logging.Debug,
			logging.Info,
			logging.Warning,
			logging.Error,
			Suppress,
		} {
			var name string
			if l == Suppress {
				name = "SUPPRESSED"
			} else {
				name = l.String()
			}
			t.Run(fmt.Sprintf(`At logging level %q`, name), func(t *ftt.Test) {
				t.Run(`Logs correctly`, func(t *ftt.Test) {
					Install(&base, l)

					// expected will accumulate during logging based on the current test
					// log level, "l".
					var expected []memlogger.LogEntry

					// Info
					grpclog.Info("info", "foo", "bar")
					grpclog.Infof("infof(%q, %q)", "foo", "bar")
					grpclog.Infoln("infoln", "foo", "bar")
					switch l {
					case Suppress, logging.Error, logging.Warning, logging.Info:
					default:
						expected = append(expected, []memlogger.LogEntry{
							{Level: logging.Debug, Msg: "info foo bar", CallDepth: 3},
							{Level: logging.Debug, Msg: `infof("foo", "bar")`, CallDepth: 3},
							{Level: logging.Debug, Msg: "infoln foo bar", CallDepth: 3},
						}...)
					}

					// Warning
					grpclog.Warning("warning", "foo", "bar")
					grpclog.Warningf("warningf(%q, %q)", "foo", "bar")
					grpclog.Warningln("warningln", "foo", "bar")
					switch l {
					case Suppress, logging.Error:
					default:
						expected = append(expected, []memlogger.LogEntry{
							{Level: logging.Warning, Msg: "warning foo bar", CallDepth: 3},
							{Level: logging.Warning, Msg: `warningf("foo", "bar")`, CallDepth: 3},
							{Level: logging.Warning, Msg: "warningln foo bar", CallDepth: 3},
						}...)
					}

					// Error
					grpclog.Error("error", "foo", "bar")
					grpclog.Errorf("errorf(%q, %q)", "foo", "bar")
					grpclog.Errorln("errorln", "foo", "bar")
					switch l {
					case Suppress:
					default:
						expected = append(expected, []memlogger.LogEntry{
							{Level: logging.Error, Msg: "error foo bar", CallDepth: 3},
							{Level: logging.Error, Msg: `errorf("foo", "bar")`, CallDepth: 3},
							{Level: logging.Error, Msg: "errorln foo bar", CallDepth: 3},
						}...)
					}

					assert.Loosely(t, base.Messages(), should.Match(expected))

					for i := 0; i < 3; i++ {
						exp := i <= translateLevel(l)
						t.Logf("Testing %q V(%d) => %v", name, i, exp)
						assert.Loosely(t, grpclog.V(i), should.Equal(exp))
					}
				})

				t.Run(`Handles Fatal calls`, func(t *ftt.Test) {
					t.Skip("XXX: disabled, pending https://github.com/grpc/grpc-go/issues/1360")

					grpclog.Fatal("fatal", "foo", "bar")
					grpclog.Fatalf("fatalf(%q, %q)", "foo", "bar")
					grpclog.Fatalln("fatalln", "foo", "bar")

					assert.Loosely(t, base.Messages(), should.Match([]memlogger.LogEntry{
						{Level: logging.Error, Msg: "fatal foo bar", CallDepth: 3},
						{Level: logging.Error, Msg: `fatalf("foo", "bar")`, CallDepth: 3},
						{Level: logging.Error, Msg: "fatalln foo bar", CallDepth: 3},
					}))
					assert.Loosely(t, exitCalls, should.Equal(3))
				})
			})
		}
	})
}
