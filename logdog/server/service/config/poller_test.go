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

package config

import (
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/logdog/api/config/svcconfig"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPoller(t *testing.T) {
	t.Parallel()

	Convey(`Using in-memory configuration manager options`, t, func() {
		c := context.Background()
		c, tc := testclock.UseTime(c, testclock.TestTimeLocal)

		// In-memory configuration service.
		cfg := &svcconfig.Config{
			Transport: &svcconfig.Transport{
				Type: &svcconfig.Transport_Pubsub{
					Pubsub: &svcconfig.Transport_PubSub{
						Project:      "foo",
						Topic:        "bar",
						Subscription: "baz",
					},
				},
			},
		}
		cset := memory.ConfigSet{
			"test-configuration.cfg": proto.MarshalTextString(cfg),
		}

		mc := memory.New(map[string]memory.ConfigSet{
			"svcconfig/logdog/test": cset,
		})
		c = testconfig.WithCommonClient(c, mc)

		p := ChangePoller{
			ConfigSet: "svcconfig/logdog/test",
			Path:      "test-configuration.cfg",
		}
		So(p.Refresh(c), ShouldBeNil)
		initHash := p.ContentHash

		Convey(`With an OnChange function installed.`, func() {
			changeDetectedC := make(chan bool, 1)
			p.Period = time.Second
			p.OnChange = func() {
				changeDetectedC <- true
			}

			timeAdvanceC := make(chan time.Duration)
			tc.SetTimerCallback(func(time.Duration, clock.Timer) {
				t, ok := <-timeAdvanceC
				if ok {
					tc.Add(t)
				}
			})

			c, cancelFunc := context.WithCancel(c)

			doneC := make(chan struct{})
			go func() {
				p.Run(c)
				close(doneC)
			}()

			var shutdownOnce sync.Once
			shutdown := func() {
				shutdownOnce.Do(func() {
					cancelFunc()
					close(timeAdvanceC)
					<-doneC
				})
			}
			defer shutdown()

			Convey(`When the configuration changes`, func() {
				cfg.Transport.GetPubsub().Project = "qux"
				cset[string(p.Path)] = proto.MarshalTextString(cfg)

				Convey(`Will execute the OnChange function if the configuration changes.`, func() {
					timeAdvanceC <- time.Second
					So(<-changeDetectedC, ShouldBeTrue)
					So(p.ContentHash, ShouldNotEqual, initHash)
				})
			})

			Convey(`Will do nothing if the configuration doesn't change.`, func() {
				// Advancing time twice ensures that the poll loop has processed at
				// least one non-changing reload.
				timeAdvanceC <- time.Second
				timeAdvanceC <- time.Second

				changeDetected := false
				select {
				case <-changeDetectedC:
					changeDetected = true
				default:
				}
				So(changeDetected, ShouldBeFalse)

				shutdown()
				So(p.ContentHash, ShouldEqual, initHash)
			})
		})
	})
}
