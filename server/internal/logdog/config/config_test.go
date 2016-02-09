// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfig(t *testing.T) {
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
		o := Options{
			Config: memory.New(map[string]memory.ConfigSet{
				"svcconfig/logdog/test": cset,
			}),
			ConfigSet:  "svcconfig/logdog/test",
			ConfigPath: "test-configuration.cfg",
		}

		Convey(`Will fail to create a Manager if the configuration does not exist.`, func() {
			o.ConfigPath = "nonexistent.cfg"

			_, err := NewManager(c, o)
			So(err, ShouldEqual, config.ErrNoConfig)
		})

		Convey(`Will fail to create a Manager if the configuration is an invalid protobuf.`, func() {
			cset[o.ConfigPath] = "not a valid text protobuf"

			_, err := NewManager(c, o)
			So(err, ShouldNotBeNil)
		})

		Convey(`Can create a Manager.`, func() {
			m, err := NewManager(c, o)
			So(err, ShouldBeNil)
			So(m.Config(), ShouldResemble, cfg)
		})

		Convey(`With a kill function installed`, func() {
			killedC := make(chan bool, 1)
			o.KillCheckInterval = time.Second
			o.KillFunc = func() {
				killedC <- true
			}

			c, cancelFunc := context.WithCancel(c)
			defer cancelFunc()

			timeAdvanceC := make(chan time.Duration)
			tc.SetTimerCallback(func(time.Duration, clock.Timer) {
				tc.Add(<-timeAdvanceC)
			})

			m, err := NewManager(c, o)
			So(err, ShouldBeNil)

			Convey(`When the configuration changes`, func() {
				cfg.Transport.GetPubsub().Project = "qux"
				cset[o.ConfigPath] = proto.MarshalTextString(cfg)

				Convey(`Will execute the kill function if the configuration changes.`, func() {
					timeAdvanceC <- time.Second
					So(<-killedC, ShouldBeTrue)
					time.Sleep(10)
				})
			})

			Convey(`Will do nothing if the configuration doesn't change.`, func() {
				// Advancing time twice ensures that the poll loop has processed at
				// least one non-changing reload.
				timeAdvanceC <- time.Second
				timeAdvanceC <- time.Second

				So(m.Config(), ShouldResemble, cfg)
			})
		})
	})
}
