// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gaeconfig

import (
	"testing"

	"github.com/luci/luci-go/server/settings"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSettingsURLToHostTranslation(t *testing.T) {
	t.Parallel()

	Convey(`A testing settings configuration`, t, func() {
		c := context.Background()

		memSettings := settings.MemoryStorage{}
		c = settings.Use(c, settings.New(&memSettings))

		Convey(`Loaded with a URL value in ConfigServiceHost`, func() {
			s := Settings{
				ConfigServiceHost: "https://example.com/foo/bar",
			}
			So(s.SetIfChanged(c, "test harness", "initial settings"), ShouldBeNil)

			Convey(`Will load the setting as a host.`, func() {
				s, err := FetchCachedSettings(c)
				So(err, ShouldBeNil)
				So(s, ShouldResemble, Settings{
					ConfigServiceHost: "example.com",
				})
			})
		})

		Convey(`Loaded with a host value in ConfigServiceHost`, func() {
			s := Settings{
				ConfigServiceHost: "example.com",
			}
			So(s.SetIfChanged(c, "test harness", "initial settings"), ShouldBeNil)

			Convey(`Will load the setting as a host.`, func() {
				s, err := FetchCachedSettings(c)
				So(err, ShouldBeNil)
				So(s, ShouldResemble, Settings{
					ConfigServiceHost: "example.com",
				})
			})
		})
	})
}
