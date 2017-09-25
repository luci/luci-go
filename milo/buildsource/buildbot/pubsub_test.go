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

package buildbot

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	memcfg "go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var (
	fakeTime    = time.Date(2001, time.February, 3, 4, 5, 6, 0, time.UTC)
	letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
)

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func newCombinedPsBody(bs []*buildbot.Build, m *buildbot.Master, internal bool) io.ReadCloser {
	bmsg := buildMasterMsg{
		Master: m,
		Builds: bs,
	}
	bm, _ := json.Marshal(bmsg)
	var b bytes.Buffer
	zw := zlib.NewWriter(&b)
	zw.Write(bm)
	zw.Close()
	sub := "projects/luci-milo/subscriptions/buildbot-public"
	if internal {
		sub = "projects/luci-milo/subscriptions/buildbot-private"
	}
	msg := common.PubSubSubscription{
		Subscription: sub,
		Message: common.PubSubMessage{
			Data: base64.StdEncoding.EncodeToString(b.Bytes()),
		},
	}
	jmsg, _ := json.Marshal(msg)
	return ioutil.NopCloser(bytes.NewReader(jmsg))
}

func TestPubSub(t *testing.T) {
	Convey(`A test Environment`, t, func() {
		c := gaetesting.TestingContextWithAppID("dev~luci-milo")
		c, _ = testclock.UseTime(c, fakeTime)
		c = testconfig.WithCommonClient(c, memcfg.New(bbAclConfigs))
		c = auth.WithState(c, &authtest.FakeState{
			Identity:       identity.AnonymousIdentity,
			IdentityGroups: []string{"all"},
		})
		// Update the service config so that the settings are loaded.
		_, err := common.UpdateServiceConfig(c)
		So(err, ShouldBeNil)

		rand.Seed(5)

		Convey("Remove source changes", func() {
			m := &buildbot.Master{
				Name: "fake",
				Builders: map[string]*buildbot.Builder{
					"fake builder": {
						PendingBuildStates: []*buildbot.Pending{
							{
								Source: buildbot.SourceStamp{
									Changes: []buildbot.Change{{Comments: "foo"}},
								},
							},
						},
					},
				},
			}
			So(putDSMasterJSON(c, m, false), ShouldBeNil)
			lm, _, _, err := getMasterJSON(c, "fake")
			So(err, ShouldBeNil)
			So(lm.Builders["fake builder"].PendingBuildStates[0].Source.Changes[0].Comments, ShouldResemble, "")
		})

		Convey("Save build entry", func() {
			build := &buildbot.Build{
				Master:      "Fake Master",
				Buildername: "Fake buildername",
				Number:      1234,
				Currentstep: "this is a string",
				Finished:    true,
			}
			err := ds.Put(c, build)
			ds.GetTestable(c).CatchupIndexes()

			So(err, ShouldBeNil)
			Convey("Load build entry", func() {
				loadB := &buildbot.Build{
					Master:      "Fake Master",
					Buildername: "Fake buildername",
					Number:      1234,
				}
				err = ds.Get(c, loadB)
				So(err, ShouldBeNil)
				So(loadB.Master, ShouldEqual, "Fake Master")
				So(loadB.Internal, ShouldEqual, false)
				So(loadB.Currentstep.(string), ShouldEqual, "this is a string")
				So(loadB.Finished, ShouldEqual, true)
			})

			Convey("Query build entry", func() {
				q := ds.NewQuery("buildbotBuild")
				buildbots := []*buildbot.Build{}
				err = ds.GetAll(c, q, &buildbots)
				So(err, ShouldBeNil)

				So(len(buildbots), ShouldEqual, 1)
				So(buildbots[0].Currentstep.(string), ShouldEqual, "this is a string")
				Convey("Query for finished entries should be 1", func() {
					q = q.Eq("finished", true)
					buildbots = []*buildbot.Build{}
					err = ds.GetAll(c, q, &buildbots)
					So(err, ShouldBeNil)
					So(len(buildbots), ShouldEqual, 1)
				})
				Convey("Query for unfinished entries should be 0", func() {
					q = q.Eq("finished", false)
					buildbots = []*buildbot.Build{}
					err = ds.GetAll(c, q, &buildbots)
					So(err, ShouldBeNil)
					So(len(buildbots), ShouldEqual, 0)
				})
			})

			Convey("Save a few more entries", func() {
				ds.GetTestable(c).Consistent(true)
				ds.GetTestable(c).AutoIndex(true)
				for i := 1235; i < 1240; i++ {
					build := &buildbot.Build{
						Master:      "Fake Master",
						Buildername: "Fake buildername",
						Number:      i,
						Currentstep: "this is a string",
						Finished:    i%2 == 0,
					}
					err := ds.Put(c, build)
					So(err, ShouldBeNil)
				}
				q := ds.NewQuery("buildbotBuild")
				q = q.Eq("finished", true)
				q = q.Eq("master", "Fake Master")
				q = q.Eq("builder", "Fake buildername")
				q = q.Order("-number")
				buildbots := []*buildbot.Build{}
				err = ds.GetAll(c, q, &buildbots)
				So(err, ShouldBeNil)
				So(len(buildbots), ShouldEqual, 3) // 1235, 1237, 1239
			})
		})

		Convey("Build panics on invalid query", func() {
			build := &buildbot.Build{
				Master: "Fake Master",
			}
			So(func() { ds.Put(c, build) }, ShouldPanicLike, "No Master or Builder found")
		})

		// FIXME: change random 123, 555, 1234 times
		// to something that makes sense and not scary to change.
		ts := unixTime(555)
		b := &buildbot.Build{
			Master:      "Fake Master",
			Buildername: "Fake buildername",
			Number:      1234,
			Currentstep: "this is a string",
			Times:       buildbot.TimeRange{Start: unixTime(123)},
			TimeStamp:   ts,
		}

		Convey("Basic master + build pusbsub subscription", func() {
			h := httptest.NewRecorder()
			slaves := map[string]*buildbot.Slave{}
			slaves["testslave"] = &buildbot.Slave{
				Name:      "testslave",
				Connected: true,
				Runningbuilds: []*buildbot.Build{
					{
						Master:      "Fake Master",
						Buildername: "Fake buildername",
						Number:      2222,
						Times:       buildbot.TimeRange{Start: unixTime(1234)},
					},
				},
			}
			ms := buildbot.Master{
				Name:     "Fake Master",
				Project:  buildbot.Project{Title: "some title"},
				Slaves:   slaves,
				Builders: map[string]*buildbot.Builder{},
			}

			ms.Builders["Fake buildername"] = &buildbot.Builder{
				CurrentBuilds: []int{1234},
			}
			r := &http.Request{
				Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, false),
			}
			p := httprouter.Params{}
			PubSubHandler(&router.Context{
				Context: c,
				Writer:  h,
				Request: r,
				Params:  p,
			})
			So(h.Code, ShouldEqual, 200)
			Convey("And stores correctly", func() {
				loadB := &buildbot.Build{
					Master:      "Fake Master",
					Buildername: "Fake buildername",
					Number:      1234,
				}
				err := ds.Get(c, loadB)
				So(err, ShouldBeNil)
				So(loadB.Master, ShouldEqual, "Fake Master")
				So(loadB.Currentstep.(string), ShouldEqual, "this is a string")
				m, _, t, err := getMasterJSON(c, "Fake Master")
				So(err, ShouldBeNil)
				So(t.Unix(), ShouldEqual, 981173106)
				So(m.Name, ShouldEqual, "Fake Master")
				So(m.Project.Title, ShouldEqual, "some title")
				So(m.Slaves["testslave"].Name, ShouldEqual, "testslave")
				So(len(m.Slaves["testslave"].Runningbuilds), ShouldEqual, 0)
				So(len(m.Slaves["testslave"].RunningbuildsMap), ShouldEqual, 1)
				So(m.Slaves["testslave"].RunningbuildsMap["Fake buildername"][0],
					ShouldEqual, 2222)
			})

			Convey("And a new master overwrites", func() {
				c, _ = testclock.UseTime(c, fakeTime.Add(time.Duration(1*time.Second)))
				ms.Project.Title = "some other title"
				h = httptest.NewRecorder()
				r := &http.Request{
					Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, false)}
				p = httprouter.Params{}
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: r,
					Params:  p,
				})
				So(h.Code, ShouldEqual, 200)
				m, _, t, err := getMasterJSON(c, "Fake Master")
				So(err, ShouldBeNil)
				So(m.Project.Title, ShouldEqual, "some other title")
				So(t.Unix(), ShouldEqual, 981173107)
				So(m.Name, ShouldEqual, "Fake Master")
			})
			Convey("And a new build overwrites", func() {
				b.Times.Start = unixTime(123)
				b.Times.Finish = unixTime(124)
				h = httptest.NewRecorder()
				r = &http.Request{
					Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, false),
				}
				p = httprouter.Params{}
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: r,
					Params:  p,
				})
				So(h.Code, ShouldEqual, 200)
				loadB := &buildbot.Build{
					Master:      "Fake Master",
					Buildername: "Fake buildername",
					Number:      1234,
				}
				err := ds.Get(c, loadB)
				So(err, ShouldBeNil)
				So(loadB.Times, ShouldResemble, b.Times)
				Convey("And another pending build is rejected", func() {
					b.Times.Start = unixTime(123)
					b.Times.Finish = buildbot.Time{}
					h = httptest.NewRecorder()
					r = &http.Request{
						Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, false),
					}
					p = httprouter.Params{}
					PubSubHandler(&router.Context{
						Context: c,
						Writer:  h,
						Request: r,
						Params:  p,
					})
					So(h.Code, ShouldEqual, 200)
					loadB := &buildbot.Build{
						Master:      "Fake Master",
						Buildername: "Fake buildername",
						Number:      1234,
					}
					err := ds.Get(c, loadB)
					So(err, ShouldBeNil)
					So(loadB.Times.Start, ShouldResemble, unixTime(123))
					So(loadB.Times.Finish, ShouldResemble, unixTime(124))
				})
			})
			Convey("Don't Expire non-existant current build under 20 min", func() {
				b.Number = 1235
				b.TimeStamp.Time = fakeTime.Add(-1000 * time.Second)
				h = httptest.NewRecorder()
				r = &http.Request{
					Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, false),
				}
				p = httprouter.Params{}
				ds.GetTestable(c).Consistent(true)
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: r,
					Params:  p,
				})
				So(h.Code, ShouldEqual, 200)
				loadB := &buildbot.Build{
					Master:      "Fake Master",
					Buildername: "Fake buildername",
					Number:      1235,
				}
				err := ds.Get(c, loadB)
				So(err, ShouldBeNil)
				So(loadB.Finished, ShouldEqual, false)
				So(loadB.Times.Start, ShouldResemble, unixTime(123))
				So(loadB.Times.Finish.IsZero(), ShouldBeTrue)
			})
			Convey("Expire non-existant current build", func() {
				b.Number = 1235
				b.TimeStamp.Time = fakeTime.Add(-1201 * time.Second)
				h = httptest.NewRecorder()
				r = &http.Request{
					Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, false),
				}
				p = httprouter.Params{}
				ds.GetTestable(c).Consistent(true)
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: r,
					Params:  p,
				})
				So(h.Code, ShouldEqual, 200)
				loadB := &buildbot.Build{
					Master:      "Fake Master",
					Buildername: "Fake buildername",
					Number:      1235,
				}
				err := ds.Get(c, loadB)
				So(err, ShouldBeNil)
				So(loadB.Finished, ShouldResemble, true)
				So(loadB.Times.Start, ShouldResemble, unixTime(123))
				So(loadB.Times.Finish, ShouldResemble, b.TimeStamp)
				So(loadB.Results, ShouldEqual, buildbot.Exception)
			})
			Convey("Large pubsub message", func() {
				// This has to be a random string, so that after gzip compresses it
				// it remains larger than 950KB
				b.Text = append(b.Text, RandStringRunes(1500000))
				h := httptest.NewRecorder()
				r := &http.Request{
					Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, false),
				}
				p := httprouter.Params{}
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: r,
					Params:  p,
				})
				So(h.Code, ShouldEqual, 200)
			})

		})

		Convey("Empty pubsub message", func() {
			h := httptest.NewRecorder()
			r := &http.Request{Body: ioutil.NopCloser(bytes.NewReader([]byte{}))}
			p := httprouter.Params{}
			PubSubHandler(&router.Context{
				Context: c,
				Writer:  h,
				Request: r,
				Params:  p,
			})
			So(h.Code, ShouldEqual, 200)
		})

		Convey("Internal master + build pusbsub subscription", func() {
			h := httptest.NewRecorder()
			slaves := map[string]*buildbot.Slave{}
			slaves["testslave"] = &buildbot.Slave{
				Name:      "testslave",
				Connected: true,
				Runningbuilds: []*buildbot.Build{
					{
						Master:      "Fake Master",
						Buildername: "Fake buildername",
						Number:      2222,
						Times:       buildbot.TimeRange{Start: unixTime(1234)},
					},
				},
			}
			ms := buildbot.Master{
				Name:    "Fake Master",
				Project: buildbot.Project{Title: "some title"},
				Slaves:  slaves,
			}
			r := &http.Request{
				Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, true),
			}
			p := httprouter.Params{}
			PubSubHandler(&router.Context{
				Context: c,
				Writer:  h,
				Request: r,
				Params:  p,
			})
			So(h.Code, ShouldEqual, 200)
			Convey("And stores correctly", func() {
				c = auth.WithState(c, &authtest.FakeState{
					Identity:       "user:alicebob@google.com",
					IdentityGroups: []string{"googlers", "all"},
				})
				loadB := &buildbot.Build{
					Master:      "Fake Master",
					Buildername: "Fake buildername",
					Number:      1234,
				}
				err = ds.Get(c, loadB)
				So(err, ShouldBeNil)
				So(loadB.Master, ShouldEqual, "Fake Master")
				So(loadB.Internal, ShouldEqual, true)
				So(loadB.Currentstep.(string), ShouldEqual, "this is a string")
				m, _, t, err := getMasterJSON(c, "Fake Master")
				So(err, ShouldBeNil)
				So(t.Unix(), ShouldEqual, 981173106)
				So(m.Name, ShouldEqual, "Fake Master")
				So(m.Project.Title, ShouldEqual, "some title")
				So(m.Slaves["testslave"].Name, ShouldEqual, "testslave")
				So(len(m.Slaves["testslave"].Runningbuilds), ShouldEqual, 0)
				So(len(m.Slaves["testslave"].RunningbuildsMap), ShouldEqual, 1)
				So(m.Slaves["testslave"].RunningbuildsMap["Fake buildername"][0],
					ShouldEqual, 2222)
			})
		})
	})
}

func unixTime(sec int64) buildbot.Time {
	return buildbot.Time{time.Unix(sec, 0).UTC()}
}
