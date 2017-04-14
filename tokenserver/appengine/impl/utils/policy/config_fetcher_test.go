// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package policy

import (
	"testing"

	"github.com/golang/protobuf/ptypes/timestamp"
	"golang.org/x/net/context"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestConfigFetcher(t *testing.T) {
	t.Parallel()

	Convey("Fetches bunch of configs", t, func() {
		c := gaetesting.TestingContext()
		c = prepareServiceConfig(c, map[string]string{
			"abc.cfg": "seconds: 12345",
			"def.cfg": "seconds: 67890",
		})

		f := luciConfigFetcher{}
		ts := timestamp.Timestamp{} // using timestamp as guinea pig proto message

		So(f.FetchTextProto(c, "missing", &ts), ShouldEqual, config.ErrNoConfig)
		So(f.Revision(), ShouldEqual, "")

		So(f.FetchTextProto(c, "abc.cfg", &ts), ShouldBeNil)
		So(ts.Seconds, ShouldEqual, 12345)
		So(f.FetchTextProto(c, "def.cfg", &ts), ShouldBeNil)
		So(ts.Seconds, ShouldEqual, 67890)

		So(f.Revision(), ShouldEqual, "cd1e0b1f602d8c639c049c9ecdc1161409a4c75b")
	})

	Convey("Revision changes midway", t, func() {
		base := gaetesting.TestingContext()

		f := luciConfigFetcher{}
		ts := timestamp.Timestamp{}

		c1 := prepareServiceConfig(base, map[string]string{
			"abc.cfg": "seconds: 12345",
		})
		So(f.FetchTextProto(c1, "abc.cfg", &ts), ShouldBeNil)
		So(ts.Seconds, ShouldEqual, 12345)

		c2 := prepareServiceConfig(base, map[string]string{
			"def.cfg": "seconds: 12345",
		})
		So(f.FetchTextProto(c2, "def.cfg", &ts), ShouldErrLike,
			`expected config "def.cfg" to be at rev 1cad281302ba31db6b55a2c91399206b29960ca8, `+
				`but got b7441146400e9980a11a7ad0d9db2068fe180670`)
	})
}

func prepareServiceConfig(c context.Context, configs map[string]string) context.Context {
	return testconfig.WithCommonClient(c, memory.New(map[string]memory.ConfigSet{
		"services/" + info.AppID(c): configs,
	}))
}
