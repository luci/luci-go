// Copyright 2018 The LUCI Authors.
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
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	structpb "github.com/golang/protobuf/ptypes/struct"

	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRegisterInstance(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx, _, _ := TestingContext()

		pkg := &Package{
			Name:         "a/b/c",
			RegisteredBy: "user:a@example.com",
			RegisteredTs: testTime,
		}

		inst := &Instance{
			InstanceID:   strings.Repeat("a", 40),
			Package:      PackageKey(ctx, "a/b/c"),
			RegisteredBy: "user:a@example.com",
			RegisteredTs: testTime,
		}

		Convey("To proto", func() {
			So(inst.Proto(), ShouldResemble, &api.Instance{
				Package: "a/b/c",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: inst.InstanceID,
				},
				RegisteredBy: "user:a@example.com",
				RegisteredTs: google.NewTimestamp(testTime),
			})
		})

		Convey("New package and instance", func() {
			reg, out, err := RegisterInstance(ctx, inst, func(c context.Context, inst *Instance) error {
				inst.ProcessorsPending = []string{"a"}
				return nil
			})
			So(err, ShouldBeNil)
			So(reg, ShouldBeTrue)

			expected := &Instance{
				InstanceID:        inst.InstanceID,
				Package:           inst.Package,
				RegisteredBy:      inst.RegisteredBy,
				RegisteredTs:      inst.RegisteredTs,
				ProcessorsPending: []string{"a"},
			}
			So(out, ShouldResemble, expected)

			// Created instance and package entities.
			storedInst := &Instance{
				InstanceID: out.InstanceID,
				Package:    inst.Package,
			}
			storedPkg := &Package{Name: "a/b/c"}
			So(datastore.Get(ctx, storedInst, storedPkg), ShouldBeNil)

			So(storedInst, ShouldResemble, expected)
			So(storedPkg, ShouldResemble, pkg)

			So(GetEvents(ctx), ShouldResembleProto, []*api.Event{
				{
					Kind:     api.EventKind_INSTANCE_CREATED,
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime.Add(1)),
					Package:  pkg.Name,
					Instance: expected.InstanceID,
				},
				{
					Kind:    api.EventKind_PACKAGE_CREATED,
					Who:     string(testUser),
					When:    google.NewTimestamp(testTime),
					Package: pkg.Name,
				},
			})
		})

		Convey("Existing package, new instance", func() {
			So(datastore.Put(ctx, pkg), ShouldBeNil)

			inst.RegisteredBy = "user:someoneelse@example.com"
			reg, out, err := RegisterInstance(ctx, inst, func(c context.Context, inst *Instance) error {
				inst.ProcessorsPending = []string{"a"}
				return nil
			})
			So(err, ShouldBeNil)
			So(reg, ShouldBeTrue)
			So(out, ShouldResemble, &Instance{
				InstanceID:        inst.InstanceID,
				Package:           inst.Package,
				RegisteredBy:      inst.RegisteredBy,
				RegisteredTs:      inst.RegisteredTs,
				ProcessorsPending: []string{"a"},
			})

			// Package entity wasn't touched.
			storedPkg := &Package{Name: "a/b/c"}
			So(datastore.Get(ctx, storedPkg), ShouldBeNil)
			So(storedPkg, ShouldResemble, pkg)

			So(GetEvents(ctx), ShouldResembleProto, []*api.Event{
				{
					Kind:     api.EventKind_INSTANCE_CREATED,
					Who:      string(testUser),
					When:     google.NewTimestamp(testTime),
					Package:  pkg.Name,
					Instance: inst.InstanceID,
				},
			})
		})

		Convey("Existing instance", func() {
			So(datastore.Put(ctx, pkg, inst), ShouldBeNil)

			modified := *inst
			modified.RegisteredBy = "user:someoneelse@example.com"
			reg, out, err := RegisterInstance(ctx, &modified, func(c context.Context, inst *Instance) error {
				panic("must not be called")
			})
			So(err, ShouldBeNil)
			So(reg, ShouldBeFalse)
			So(out, ShouldResemble, inst) // the original one

			So(GetEvents(ctx), ShouldHaveLength, 0)
		})
	})
}

func TestListInstances(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx, _, _ := TestingContext()

		inst := func(i int) *Instance {
			return &Instance{
				InstanceID:   fmt.Sprintf("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa%d", i),
				Package:      PackageKey(ctx, "a/b"),
				RegisteredTs: testTime.Add(time.Duration(i) * time.Minute),
			}
		}
		for i := 0; i < 4; i++ {
			So(datastore.Put(ctx, inst(i)), ShouldBeNil)
		}

		Convey("Full listing", func() {
			out, cur, err := ListInstances(ctx, "a/b", 100, nil)
			So(err, ShouldBeNil)
			So(cur, ShouldBeNil)
			So(out, ShouldResemble, []*Instance{inst(3), inst(2), inst(1), inst(0)})
		})

		Convey("Paginated listing", func() {
			out, cur, err := ListInstances(ctx, "a/b", 3, nil)
			So(err, ShouldBeNil)
			So(cur, ShouldNotBeNil)
			So(out, ShouldResemble, []*Instance{inst(3), inst(2), inst(1)})

			out, cur, err = ListInstances(ctx, "a/b", 3, cur)
			So(err, ShouldBeNil)
			So(cur, ShouldBeNil)
			So(out, ShouldResemble, []*Instance{inst(0)})
		})
	})
}

func TestCheckInstance(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx, _, _ := TestingContext()

		put := func(pkg, iid string, failedProcs, pendingProcs []string) {
			So(datastore.Put(ctx,
				&Package{Name: pkg},
				&Instance{
					InstanceID:        iid,
					Package:           PackageKey(ctx, pkg),
					ProcessorsFailure: failedProcs,
					ProcessorsPending: pendingProcs,
				}), ShouldBeNil)
		}

		iid := common.ObjectRefToInstanceID(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: strings.Repeat("a", 40),
		})
		inst := &Instance{
			InstanceID: iid,
			Package:    PackageKey(ctx, "pkg"),
		}

		Convey("Happy path", func() {
			put("pkg", iid, nil, nil)
			So(CheckInstanceExists(ctx, inst), ShouldBeNil)
			So(CheckInstanceReady(ctx, inst), ShouldBeNil)
		})

		Convey("No such instance", func() {
			put("pkg", "f"+iid[1:], nil, nil)

			err := CheckInstanceExists(ctx, inst)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such instance")

			err = CheckInstanceReady(ctx, inst)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such instance")
		})

		Convey("No such package", func() {
			put("pkg2", iid, nil, nil)

			err := CheckInstanceExists(ctx, inst)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such package")

			err = CheckInstanceReady(ctx, inst)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
			So(err, ShouldErrLike, "no such package")
		})

		Convey("Failed processors", func() {
			put("pkg", iid, []string{"f1", "f2"}, []string{"p1", "p2"})
			err := CheckInstanceReady(ctx, inst)
			So(grpcutil.Code(err), ShouldEqual, codes.Aborted)
			So(err, ShouldErrLike, "some processors failed to process this instance: f1, f2")

			So(CheckInstanceExists(ctx, inst), ShouldBeNil) // doesn't care
		})

		Convey("Pending processors", func() {
			put("pkg", iid, nil, []string{"p1", "p2"})
			err := CheckInstanceReady(ctx, inst)
			So(grpcutil.Code(err), ShouldEqual, codes.FailedPrecondition)
			So(err, ShouldErrLike, "the instance is not ready yet, pending processors: p1, p2")

			So(CheckInstanceExists(ctx, inst), ShouldBeNil) // doesn't care
		})
	})
}

func TestFetchProcessors(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx, _, _ := TestingContext()
		ts := time.Unix(1525136124, 0).UTC()

		inst := &Instance{
			InstanceID:        strings.Repeat("a", 40),
			Package:           PackageKey(ctx, "a/b"),
			ProcessorsPending: []string{"p2", "p1"},
			ProcessorsFailure: []string{"f2", "f1"},
			ProcessorsSuccess: []string{"s2", "s1"},
		}

		proc := func(id string, res, err string) *ProcessingResult {
			p := &ProcessingResult{
				ProcID:    id,
				Instance:  datastore.KeyForObj(ctx, inst),
				CreatedTs: ts,
			}
			if res != "" {
				p.Success = true
				So(p.WriteResult(map[string]string{"result": res}), ShouldBeNil)
			} else {
				p.Error = err
			}
			return p
		}

		So(datastore.Put(ctx,
			proc("f1", "", "fail 1"),
			proc("f2", "", "fail 2"),
			proc("s1", "success 1", ""),
			proc("s2", "success 2", ""),
		), ShouldBeNil)

		Convey("Works", func() {
			procs, err := FetchProcessors(ctx, inst)
			So(err, ShouldBeNil)
			So(procs, ShouldResembleProto, []*api.Processor{
				{
					Id:         "f1",
					State:      api.Processor_FAILED,
					FinishedTs: google.NewTimestamp(ts),
					Error:      "fail 1",
				},
				{
					Id:         "f2",
					State:      api.Processor_FAILED,
					FinishedTs: google.NewTimestamp(ts),
					Error:      "fail 2",
				},
				{
					Id:    "p1",
					State: api.Processor_PENDING,
				},
				{
					Id:    "p2",
					State: api.Processor_PENDING,
				},
				{
					Id:         "s1",
					State:      api.Processor_SUCCEEDED,
					FinishedTs: google.NewTimestamp(ts),
					Result: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"result": {Kind: &structpb.Value_StringValue{StringValue: "success 1"}},
						},
					},
				},
				{
					Id:         "s2",
					State:      api.Processor_SUCCEEDED,
					FinishedTs: google.NewTimestamp(ts),
					Result: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"result": {Kind: &structpb.Value_StringValue{StringValue: "success 2"}},
						},
					},
				},
			})
		})
	})
}
