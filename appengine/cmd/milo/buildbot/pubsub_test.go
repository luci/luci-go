// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbot

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func buildbotTimesFinished(start, end float64) []*float64 {
	return []*float64{&start, &end}
}

func buildbotTimesPending(start float64) []*float64 {
	return []*float64{&start, nil}
}

func newPsBody(bs []buildbotBuild) io.ReadCloser {
	bm, _ := json.Marshal(bs)
	msg := pubSubSubscription{
		Subscription: "projects/luci-milo/subscriptions/buildbot-public",
		Message: pubSubMessage{
			Data: base64.StdEncoding.EncodeToString(bm),
		},
	}
	jmsg, _ := json.Marshal(msg)
	return ioutil.NopCloser(bytes.NewReader(jmsg))
}

func TestPubSub(t *testing.T) {
	Convey(`A test Environment`, t, func() {
		c := memory.Use(context.Background())
		c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
		ds := datastore.Get(c)

		Convey("Save build entry", func() {
			build := &buildbotBuild{
				Master:      "Fake Master",
				Buildername: "Fake buildername",
				Number:      1234,
				Currentstep: "this is a string",
			}
			err := ds.Put(build)
			ds.Testable().CatchupIndexes()

			So(err, ShouldBeNil)
			Convey("Load build entry", func() {
				loadB := &buildbotBuild{
					Master:      "Fake Master",
					Buildername: "Fake buildername",
					Number:      1234,
				}
				err = ds.Get(loadB)
				So(err, ShouldBeNil)
				So(loadB.Master, ShouldEqual, "Fake Master")
				So(loadB.Currentstep.(string), ShouldEqual, "this is a string")
			})

			Convey("Query build entry", func() {
				q := datastore.NewQuery("buildbotBuild")
				buildbots := []*buildbotBuild{}
				err = ds.GetAll(q, &buildbots)
				So(err, ShouldBeNil)

				So(len(buildbots), ShouldEqual, 1)
				So(buildbots[0].Currentstep.(string), ShouldEqual, "this is a string")
			})
		})

		Convey("Build panics on invalid query", func() {
			build := &buildbotBuild{
				Master: "Fake Master",
			}
			So(func() { ds.Put(build) }, ShouldPanicLike, "No Master or Builder found")
		})

		Convey("Basic pusbsub subscription", func() {
			b := buildbotBuild{
				Master:      "Fake Master",
				Buildername: "Fake buildername",
				Number:      1234,
				Currentstep: "this is a string",
				Times:       buildbotTimesPending(123.0),
			}
			h := httptest.NewRecorder()
			r := &http.Request{Body: newPsBody([]buildbotBuild{b})}
			p := httprouter.Params{}
			PubSubHandler(c, h, r, p)
			So(h.Code, ShouldEqual, 200)
			Convey("And stores correctly", func() {
				loadB := &buildbotBuild{
					Master:      "Fake Master",
					Buildername: "Fake buildername",
					Number:      1234,
				}
				err := ds.Get(loadB)
				So(err, ShouldBeNil)
				So(loadB.Master, ShouldEqual, "Fake Master")
				So(loadB.Currentstep.(string), ShouldEqual, "this is a string")
			})
			Convey("And a new build overwrites", func() {
				b.Times = buildbotTimesFinished(123.0, 124.0)
				h = httptest.NewRecorder()
				r = &http.Request{Body: newPsBody([]buildbotBuild{b})}
				p = httprouter.Params{}
				PubSubHandler(c, h, r, p)
				So(h.Code, ShouldEqual, 200)
				loadB := &buildbotBuild{
					Master:      "Fake Master",
					Buildername: "Fake buildername",
					Number:      1234,
				}
				err := ds.Get(loadB)
				So(err, ShouldBeNil)
				So(*loadB.Times[0], ShouldEqual, 123.0)
				So(*loadB.Times[1], ShouldEqual, 124.0)
				Convey("And another pending build is rejected", func() {
					b.Times = buildbotTimesPending(123.0)
					h = httptest.NewRecorder()
					r = &http.Request{Body: newPsBody([]buildbotBuild{b})}
					p = httprouter.Params{}
					PubSubHandler(c, h, r, p)
					So(h.Code, ShouldEqual, 200)
					loadB := &buildbotBuild{
						Master:      "Fake Master",
						Buildername: "Fake buildername",
						Number:      1234,
					}
					err := ds.Get(loadB)
					So(err, ShouldBeNil)
					So(*loadB.Times[0], ShouldEqual, 123.0)
					So(*loadB.Times[1], ShouldEqual, 124.0)
				})
			})
		})
	})
}
