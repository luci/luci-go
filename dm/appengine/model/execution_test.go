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

package model

import (
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/proto/google"
	. "go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/dm/api/service/v1"
)

func TestExecutions(t *testing.T) {
	t.Parallel()

	Convey("Execution", t, func() {
		c := memory.Use(context.Background())
		c = memlogger.Use(c)

		a := &Attempt{ID: *dm.NewAttemptID("q", 1)}
		ak := ds.KeyForObj(c, a)

		Convey("Revoke", func() {
			e1 := &Execution{ID: 1, Attempt: ak, Token: []byte("good tok"), State: dm.Execution_RUNNING}
			So(ds.Put(c, e1), ShouldBeNil)

			So(ds.KeyForObj(c, e1).String(), ShouldEqual,
				`dev~app::/Attempt,"q|fffffffe"/Execution,"fffffffe"`)

			e2 := *e1
			So(e2.Revoke(c), ShouldBeNil)

			So(e1.Token, ShouldResemble, []byte("good tok"))
			So(ds.Get(c, e1), ShouldBeNil)
			So(e1.Token, ShouldBeNil)
		})

		Convey("Verify", func() {
			e1 := &Execution{ID: 1, Attempt: ak, Token: []byte("good tok")}
			So(ds.Put(c, e1), ShouldBeNil)

			auth := &dm.Execution_Auth{
				Id:    dm.NewExecutionID("q", a.ID.Id, uint32(e1.ID)),
				Token: []byte("bad tok"),
			}

			_, _, err := AuthenticateExecution(c, auth)
			So(err, ShouldBeRPCInternal, "execution Auth")

			So(ds.Put(c, a), ShouldBeNil)
			_, _, err = AuthenticateExecution(c, auth)
			So(err, ShouldBeRPCPermissionDenied, "execution Auth")

			a.CurExecution = 1
			So(ds.Put(c, a), ShouldBeNil)
			_, _, err = AuthenticateExecution(c, auth)
			So(err, ShouldBeRPCPermissionDenied, "execution Auth")

			a.State = dm.Attempt_EXECUTING
			So(ds.Put(c, a), ShouldBeNil)
			_, _, err = AuthenticateExecution(c, auth)
			So(err, ShouldBeRPCPermissionDenied, "execution Auth")

			e1.State = dm.Execution_RUNNING
			So(ds.Put(c, e1), ShouldBeNil)
			_, _, err = AuthenticateExecution(c, auth)
			So(err, ShouldBeRPCPermissionDenied, "execution Auth")

			auth.Token = []byte("good tok")
			atmpt, exe, err := AuthenticateExecution(c, auth)
			So(err, ShouldBeNil)

			So(atmpt, ShouldResemble, a)
			So(exe, ShouldResemble, e1)
		})

		Convey("Activate", func() {
			e1 := &Execution{
				ID:      1,
				Attempt: ak,
				Token:   []byte("good tok"),
			}
			a.CurExecution = 1
			So(ds.Put(c, a, e1), ShouldBeNil)

			auth := &dm.Execution_Auth{
				Id:    dm.NewExecutionID("q", a.ID.Id, uint32(e1.ID)),
				Token: []byte("wrong tok"),
			}

			Convey("wrong execution id", func() {
				auth.Id.Id++
				_, _, err := ActivateExecution(c, auth, []byte("wrong tok"))
				So(err, ShouldBeRPCInternal, "execution Auth")
			})

			Convey("attempt bad state", func() {
				_, _, err := ActivateExecution(c, auth, []byte("wrong tok"))
				So(err, ShouldBeRPCPermissionDenied, "execution Auth")
			})

			Convey("attempt executing", func() {
				a.State = dm.Attempt_EXECUTING
				So(ds.Put(c, a), ShouldBeNil)

				Convey("wrong execution state", func() {
					e1.State = dm.Execution_STOPPING
					So(ds.Put(c, e1), ShouldBeNil)
					_, _, err := ActivateExecution(c, auth, []byte("wrong token"))
					So(err, ShouldBeRPCPermissionDenied, "execution Auth")
				})

				Convey("wrong token", func() {
					_, _, err := ActivateExecution(c, auth, []byte("wrong tok"))
					So(err, ShouldBeRPCPermissionDenied, "execution Auth")
				})

				Convey("correct token", func() {
					auth.Token = []byte("good tok")
					newA, e, err := ActivateExecution(c, auth, []byte("new token"))
					So(err, ShouldBeNil)
					So(newA, ShouldResemble, a)
					So(e.State, ShouldEqual, dm.Execution_RUNNING)

					Convey("retry with different token fails", func() {
						_, _, err = ActivateExecution(c, auth, []byte("other token"))
						So(err, ShouldBeRPCPermissionDenied, "execution Auth")
					})

					Convey("retry with same token OK", func() {
						auth.Token = []byte("new token")
						_, _, err = ActivateExecution(c, auth, []byte("new token"))
						So(err, ShouldBeNil)
					})
				})
			})

		})

		Convey("Invalidate", func() {
			e1 := &Execution{
				ID:      1,
				Attempt: ak,
				Token:   []byte("good tok"),
				State:   dm.Execution_RUNNING,
			}
			So(ds.Put(c, e1), ShouldBeNil)

			a.CurExecution = 1
			a.State = dm.Attempt_EXECUTING
			So(ds.Put(c, a), ShouldBeNil)

			auth := &dm.Execution_Auth{
				Id:    dm.NewExecutionID("q", a.ID.Id, uint32(e1.ID)),
				Token: []byte("bad token"),
			}

			_, _, err := InvalidateExecution(c, auth)
			So(err, ShouldBeRPCPermissionDenied, "execution Auth")

			auth.Token = []byte("good tok")
			_, _, err = InvalidateExecution(c, auth)
			So(err, ShouldBeNil)

			So(ds.Get(c, e1), ShouldBeNil)
			So(e1.Token, ShouldBeNil)

			_, _, err = InvalidateExecution(c, auth)
			So(err, ShouldBeRPCPermissionDenied, "requires execution Auth")
		})

		Convey("failed invalidation", func() {
			e1 := &Execution{
				ID:      1,
				Attempt: ak,
				Token:   []byte("good tok"),
				State:   dm.Execution_RUNNING,
			}
			So(ds.Put(c, e1), ShouldBeNil)
			a.CurExecution = 1
			a.State = dm.Attempt_EXECUTING
			So(ds.Put(c, a), ShouldBeNil)

			auth := &dm.Execution_Auth{
				Id:    dm.NewExecutionID("q", a.ID.Id, uint32(e1.ID)),
				Token: []byte("good tok"),
			}

			c, fb := featureBreaker.FilterRDS(c, nil)
			fb.BreakFeatures(nil, "PutMulti")

			_, _, err := InvalidateExecution(c, auth)
			So(err, ShouldBeRPCPermissionDenied, "unable to invalidate Auth")

			fb.UnbreakFeatures("PutMulti")

			_, _, err = InvalidateExecution(c, auth)
			So(err, ShouldBeNil)

			So(ds.Get(c, e1), ShouldBeNil)
			So(e1.Token, ShouldBeNil)
		})
	})

}

func TestExecutionToProto(t *testing.T) {
	t.Parallel()

	Convey("Test Execution.ToProto", t, func() {
		c := memory.Use(context.Background())
		c = memlogger.Use(c)

		e := &Execution{
			ID:      1,
			Attempt: ds.MakeKey(c, "Attempt", "qst|fffffffe"),

			Created:          testclock.TestTimeUTC,
			Modified:         testclock.TestTimeUTC,
			DistributorToken: "id",

			Token: []byte("secret"),
		}

		Convey("no id", func() {
			exp := dm.NewExecutionScheduling()
			exp.Data.Created = google.NewTimestamp(testclock.TestTimeUTC)
			exp.Data.Modified = google.NewTimestamp(testclock.TestTimeUTC)
			exp.Data.DistributorInfo = &dm.Execution_Data_DistributorInfo{Token: "id"}

			So(e.ToProto(false), ShouldResemble, exp)
		})

		Convey("with id", func() {
			exp := dm.NewExecutionScheduling()
			exp.Id = dm.NewExecutionID("qst", 1, 1)
			exp.Data.Created = google.NewTimestamp(testclock.TestTimeUTC)
			exp.Data.Modified = google.NewTimestamp(testclock.TestTimeUTC)
			exp.Data.DistributorInfo = &dm.Execution_Data_DistributorInfo{Token: "id"}

			So(e.ToProto(true), ShouldResemble, exp)
		})
	})
}
