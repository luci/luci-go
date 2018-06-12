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

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/router"

	. "github.com/smartystreets/goconvey/convey"
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
	t.Parallel()

	Convey(`A test Environment`, t, func() {
		c := gaetesting.TestingContextWithAppID("dev~luci-milo")
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)
		c, _ = testclock.UseTime(c, fakeTime)
		c = testconfig.WithCommonClient(c, memcfg.New(bbACLConfigs))
		c = auth.WithState(c, &authtest.FakeState{
			Identity:       identity.AnonymousIdentity,
			IdentityGroups: []string{"all"},
		})
		c = caching.WithRequestCache(c)
		// Update the service config so that the settings are loaded.
		_, err := common.UpdateServiceConfig(c)
		So(err, ShouldBeNil)

		err = buildstore.SaveMaster(c, &buildbot.Master{Name: "Fake buildbotMasterEntry"}, false, nil)
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
			So(buildstore.SaveMaster(c, m, false, nil), ShouldBeNil)
			lm, err := buildstore.GetMaster(c, "fake", false)
			So(err, ShouldBeNil)
			So(lm.Builders["fake builder"].PendingBuildStates[0].Source.Changes[0].Comments, ShouldResemble, "")
		})

		Convey("Save build entry", func() {
			build := &buildbot.Build{
				Master:      "Fake buildbotMasterEntry",
				Buildername: "Fake buildername",
				Number:      1234,
				Currentstep: "this is a string",
				Finished:    true,
			}
			importBuild(c, build)

			So(err, ShouldBeNil)
			Convey("Load build entry", func() {
				loadB, err := buildstore.GetBuild(c, buildbot.BuildID{
					Master:  "Fake buildbotMasterEntry",
					Builder: "Fake buildername",
					Number:  1234,
				})
				So(err, ShouldBeNil)
				So(loadB.Master, ShouldEqual, "Fake buildbotMasterEntry")
				So(loadB.Internal, ShouldEqual, false)
				So(loadB.Currentstep, ShouldEqual, "this is a string")
				So(loadB.Finished, ShouldEqual, true)
			})

			Convey("Query build entry", func() {
				datastore.GetTestable(c).CatchupIndexes()
				q := buildstore.Query{
					Master:  build.Master,
					Builder: build.Buildername,
				}
				res, err := buildstore.GetBuilds(c, q)
				So(err, ShouldBeNil)
				So(len(res.Builds), ShouldEqual, 1)
				So(res.Builds[0].Currentstep, ShouldEqual, "this is a string")

				Convey("Query for finished entries should be 1", func() {
					q := q
					q.Finished = buildstore.Yes
					res, err := buildstore.GetBuilds(c, q)
					So(err, ShouldBeNil)
					So(len(res.Builds), ShouldEqual, 1)
					So(res.Builds[0].Currentstep, ShouldEqual, "this is a string")
				})
				Convey("Query for unfinished entries should be 0", func() {
					q := q
					q.Finished = buildstore.No
					res, err := buildstore.GetBuilds(c, q)
					So(err, ShouldBeNil)
					So(len(res.Builds), ShouldEqual, 0)
				})
			})

			Convey("Save a few more entries", func() {
				for i := 1235; i < 1240; i++ {
					importBuild(c, &buildbot.Build{
						Master:      "Fake buildbotMasterEntry",
						Buildername: "Fake buildername",
						Number:      i,
						Currentstep: "this is a string",
						Finished:    i%2 == 0,
					})
					So(err, ShouldBeNil)
				}
				res, err := buildstore.GetBuilds(c, buildstore.Query{
					Master:   "Fake buildbotMasterEntry",
					Builder:  "Fake buildername",
					Finished: buildstore.Yes,
				})
				So(err, ShouldBeNil)
				So(len(res.Builds), ShouldEqual, 3) // 1235, 1237, 1239
			})
		})

		// FIXME: change random 123, 555, 1234 times
		// to something that makes sense and not scary to change.
		ts := unixTime(555)
		b := &buildbot.Build{
			Master:      "Fake buildbotMasterEntry",
			Buildername: "Fake buildername",
			Number:      1234,
			Currentstep: "this is a string",
			Times:       buildbot.TimeRange{Start: unixTime(123)},
			TimeStamp:   ts,
			ViewPath:    "/buildbot/Fake buildbotMasterEntry/Fake buildername/1234",
		}

		bDone := &buildbot.Build{
			Master:      "Fake buildbotMasterEntry",
			Buildername: "Fake buildername",
			Number:      2234,
			Results:     buildbot.Failure,
			Times:       buildbot.TimeRange{Start: unixTime(8123), Finish: unixTime(8124)},
			TimeStamp:   ts,
		}

		builderSummary := &model.BuilderSummary{
			BuilderID: "buildbot/Fake buildbotMasterEntry/Fake buildername",
		}
		datastore.Put(c, builderSummary)
		datastore.GetTestable(c).CatchupIndexes()

		Convey("Basic master + build pusbsub subscription", func() {
			h := httptest.NewRecorder()
			slaves := map[string]*buildbot.Slave{}
			slaves["testslave"] = &buildbot.Slave{
				Name:      "testslave",
				Connected: true,
				Runningbuilds: []*buildbot.Build{
					{
						Master:      "Fake buildbotMasterEntry",
						Buildername: "Fake buildername",
						Number:      2222,
						Results:     buildbot.NoResult, // some non-terminal result
						Times:       buildbot.TimeRange{Start: unixTime(1234)},
					},
				},
			}
			ms := buildbot.Master{
				Name:     "Fake buildbotMasterEntry",
				Project:  buildbot.Project{Title: "some title"},
				Slaves:   slaves,
				Builders: map[string]*buildbot.Builder{},
			}

			ms.Builders["Fake buildername"] = &buildbot.Builder{
				CurrentBuilds: []int{1234},
			}

			r := &http.Request{
				Body: newCombinedPsBody([]*buildbot.Build{b, bDone}, &ms, false),
			}
			PubSubHandler(&router.Context{
				Context: c,
				Writer:  h,
				Request: r,
			})
			So(h.Code, ShouldEqual, 200)

			Convey("And updates builder summary correctly", func() {
				builderSummary := &model.BuilderSummary{
					BuilderID: "buildbot/Fake buildbotMasterEntry/Fake buildername",
				}
				err = datastore.Get(c, builderSummary)
				So(err, ShouldBeNil)

				So(builderSummary.LastFinishedCreated, ShouldResemble, unixTime(8123).Time)
				So(builderSummary.LastFinishedStatus, ShouldResemble, model.Failure)
				So(builderSummary.LastFinishedBuildID, ShouldEqual, "buildbot/Fake buildbotMasterEntry/Fake buildername/2234")
			})

			Convey("And stores correctly", func() {
				loadB, err := buildstore.GetBuild(c, buildbot.BuildID{
					Master:  b.Master,
					Builder: b.Buildername,
					Number:  b.Number,
				})
				So(err, ShouldBeNil)
				So(loadB.Master, ShouldEqual, "Fake buildbotMasterEntry")
				So(loadB.Currentstep, ShouldEqual, "this is a string")
				m, err := buildstore.GetMaster(c, "Fake buildbotMasterEntry", false)
				So(err, ShouldBeNil)
				So(m.Modified.Unix(), ShouldEqual, 981173106)
				So(m.Name, ShouldEqual, "Fake buildbotMasterEntry")
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
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: r,
				})
				So(h.Code, ShouldEqual, 200)
				m, err := buildstore.GetMaster(c, "Fake buildbotMasterEntry", false)
				So(err, ShouldBeNil)
				So(m.Project.Title, ShouldEqual, "some other title")
				So(m.Modified.Unix(), ShouldEqual, 981173107)
				So(m.Name, ShouldEqual, "Fake buildbotMasterEntry")
			})
			Convey("And a new build overwrites", func() {
				b.Times.Start = unixTime(123)
				b.Times.Finish = unixTime(124)
				h = httptest.NewRecorder()
				r = &http.Request{
					Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, false),
				}
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: r,
				})
				So(h.Code, ShouldEqual, 200)
				loadB, err := buildstore.GetBuild(c, buildbot.BuildID{
					Master:  "Fake buildbotMasterEntry",
					Builder: "Fake buildername",
					Number:  1234,
				})
				So(err, ShouldBeNil)
				So(loadB.Times, ShouldResemble, b.Times)

				Convey("And another pending build is rejected", func() {
					b.Times.Start = unixTime(123)
					b.Times.Finish = buildbot.Time{}
					h = httptest.NewRecorder()
					r = &http.Request{
						Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, false),
					}
					PubSubHandler(&router.Context{
						Context: c,
						Writer:  h,
						Request: r,
					})
					So(h.Code, ShouldEqual, 200)
					loadB, err := buildstore.GetBuild(c, buildbot.BuildID{
						Master:  "Fake buildbotMasterEntry",
						Builder: "Fake buildername",
						Number:  1234,
					})
					So(err, ShouldBeNil)
					So(loadB.Times.Start, ShouldResemble, unixTime(123))
					So(loadB.Times.Finish, ShouldResemble, unixTime(124))
				})
			})

			Convey("Don't Expire non-existent current build under 20 min", func() {
				b.Number = 1235
				b.TimeStamp.Time = fakeTime.Add(-1000 * time.Second)
				h = httptest.NewRecorder()
				r = &http.Request{
					Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, false),
				}
				datastore.GetTestable(c).Consistent(true)
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: r,
				})
				So(h.Code, ShouldEqual, 200)
				loadB, err := buildstore.GetBuild(c, buildbot.BuildID{
					Master:  "Fake buildbotMasterEntry",
					Builder: "Fake buildername",
					Number:  1235,
				})
				So(err, ShouldBeNil)
				So(loadB.Finished, ShouldEqual, false)
				So(loadB.Times.Start, ShouldResemble, unixTime(123))
				So(loadB.Times.Finish.IsZero(), ShouldBeTrue)
			})
			Convey("Expire non-existent current build", func() {
				b.Number = 1235
				b.TimeStamp.Time = fakeTime.Add(-21 * time.Minute)
				h = httptest.NewRecorder()
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: &http.Request{
						Body: newCombinedPsBody([]*buildbot.Build{b}, &ms, false),
					},
				})
				So(h.Code, ShouldEqual, 200)
				loadB, err := buildstore.GetBuild(c, buildbot.BuildID{
					Master:  "Fake buildbotMasterEntry",
					Builder: "Fake buildername",
					Number:  1235,
				})
				So(err, ShouldBeNil)
				So(loadB.Finished, ShouldBeTrue)
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
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: r,
				})
				So(h.Code, ShouldEqual, 200)
			})

		})

		Convey("Empty pubsub message", func() {
			h := httptest.NewRecorder()
			r := &http.Request{Body: ioutil.NopCloser(bytes.NewReader([]byte{}))}
			PubSubHandler(&router.Context{
				Context: c,
				Writer:  h,
				Request: r,
			})
			So(h.Code, ShouldEqual, 200)
		})

		Convey("Internal master + build pubsub subscription", func() {
			h := httptest.NewRecorder()
			slaves := map[string]*buildbot.Slave{}
			slaves["testslave"] = &buildbot.Slave{
				Name:      "testslave",
				Connected: true,
				Runningbuilds: []*buildbot.Build{
					{
						Master:      "Fake buildbotMasterEntry",
						Buildername: "Fake buildername",
						Number:      2222,
						Results:     buildbot.NoResult, // some non-terminal result
						Times:       buildbot.TimeRange{Start: unixTime(1234)},
					},
				},
			}
			ms := buildbot.Master{
				Name:    "Fake buildbotMasterEntry",
				Project: buildbot.Project{Title: "some title"},
				Slaves:  slaves,
				Builders: map[string]*buildbot.Builder{
					b.Buildername: {
						CurrentBuilds: []int{b.Number},
					},
				},
			}
			b.Internal = true
			PubSubHandler(&router.Context{
				Context: c,
				Writer:  h,
				Request: &http.Request{
					Body: newCombinedPsBody([]*buildbot.Build{b, bDone}, &ms, true),
				},
			})
			So(h.Code, ShouldEqual, http.StatusOK)

			Convey("And updates builder summary correctly", func() {
				builderSummary := &model.BuilderSummary{
					BuilderID: "buildbot/Fake buildbotMasterEntry/Fake buildername",
				}
				err = datastore.Get(c, builderSummary)
				So(err, ShouldBeNil)

				So(builderSummary.LastFinishedCreated, ShouldResemble, unixTime(8123).Time)
				So(builderSummary.LastFinishedStatus, ShouldResemble, model.Failure)
				So(builderSummary.LastFinishedBuildID, ShouldEqual, "buildbot/Fake buildbotMasterEntry/Fake buildername/2234")
			})

			Convey("And stores correctly", func() {
				c = auth.WithState(c, &authtest.FakeState{
					Identity:       "user:alicebob@google.com",
					IdentityGroups: []string{"googlers", "all"},
				})
				loadB, err := buildstore.GetBuild(c, buildbot.BuildID{
					Master:  b.Master,
					Builder: b.Buildername,
					Number:  b.Number,
				})
				So(err, ShouldBeNil)
				So(loadB, ShouldResemble, b)

				m, err := buildstore.GetMaster(c, "Fake buildbotMasterEntry", false)
				So(err, ShouldBeNil)
				So(m.Modified.Unix(), ShouldEqual, 981173106)
				So(m.Name, ShouldEqual, "Fake buildbotMasterEntry")
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
	return buildbot.Time{Time: time.Unix(sec, 0).UTC()}
}
