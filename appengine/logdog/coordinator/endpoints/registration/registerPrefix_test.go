// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package registration

import (
	"errors"
	"testing"

	"github.com/luci/gae/filter/featureBreaker"
	ds "github.com/luci/gae/service/datastore"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/appengine/logdog/coordinator/hierarchy"
	"github.com/luci/luci-go/common/api/logdog_coordinator/registration/v1"
	"github.com/luci/luci-go/common/cryptorand"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRegisterPrefix(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, _ := ct.Install()
		c, fb := featureBreaker.FilterRDS(c, nil)
		ds.Get(c).Testable().Consistent(true)

		// Mock random number generator so we can predict secrets.
		c = cryptorand.MockForTest(c, 0)
		randSecret := []byte{
			250, 18, 249, 42, 251, 224, 15, 133, 8, 208, 232, 59,
			171, 156, 248, 206, 191, 66, 226, 94, 139, 20, 234, 252,
			129, 234, 224, 208, 15, 44, 173, 228, 193, 124, 22, 209,
		}

		svr := New()

		req := logdog.RegisterPrefixRequest{
			Project:    "proj-foo",
			Prefix:     "testing/prefix",
			SourceInfo: []string{"unit test"},
		}

		Convey(`Returns PermissionDenied error if not user does not have write access.`, func() {
			// "proj-bar" does not have anonymous write.
			req.Project = "proj-bar"

			_, err := svr.RegisterPrefix(c, &req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`Will register a new prefix.`, func() {
			resp, err := svr.RegisterPrefix(c, &req)
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &logdog.RegisterPrefixResponse{
				LogBundleTopic: "projects/app/topics/test-topic",
				Secret:         randSecret,
			})

			// Should have registered path components.
			getComponents := func(b string) []string {
				l, err := hierarchy.Get(c, hierarchy.Request{Project: "proj-foo", PathBase: b})
				if err != nil {
					panic(err)
				}
				names := make([]string, len(l.Comp))
				for i, e := range l.Comp {
					names[i] = e.Name
				}
				return names
			}
			So(getComponents(""), ShouldResemble, []string{"testing"})
			So(getComponents("testing"), ShouldResemble, []string{"prefix"})
			So(getComponents("testing/prefix"), ShouldResemble, []string{"+"})
			So(getComponents("testing/prefix/+"), ShouldResemble, []string{})

			Convey(`Will refuse to register it again.`, func() {
				_, err := svr.RegisterPrefix(c, &req)
				So(err, ShouldBeRPCAlreadyExists)
			})
		})

		Convey(`Will fail to register the prefix if Put is broken.`, func() {
			fb.BreakFeatures(errors.New("test error"), "PutMulti")
			_, err := svr.RegisterPrefix(c, &req)
			So(err, ShouldBeRPCInternal)
		})
	})
}
