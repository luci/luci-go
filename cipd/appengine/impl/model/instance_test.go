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

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
)

func TestRegisterInstance(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		pkg := &Package{
			Name:         "a/b/c",
			RegisteredBy: "user:a@example.com",
			RegisteredTs: testutil.TestTime,
		}

		inst := &Instance{
			InstanceID:   strings.Repeat("a", 40),
			Package:      PackageKey(ctx, "a/b/c"),
			RegisteredBy: "user:a@example.com",
			RegisteredTs: testutil.TestTime,
		}

		t.Run("To proto", func(t *ftt.Test) {
			assert.Loosely(t, inst.Proto(), should.Resemble(&api.Instance{
				Package: "a/b/c",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: inst.InstanceID,
				},
				RegisteredBy: "user:a@example.com",
				RegisteredTs: timestamppb.New(testutil.TestTime),
			}))
		})

		t.Run("New package and instance", func(t *ftt.Test) {
			reg, out, err := RegisterInstance(ctx, inst, func(ctx context.Context, inst *Instance) error {
				inst.ProcessorsPending = []string{"a"}
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, reg, should.BeTrue)

			expected := &Instance{
				InstanceID:        inst.InstanceID,
				Package:           inst.Package,
				RegisteredBy:      inst.RegisteredBy,
				RegisteredTs:      inst.RegisteredTs,
				ProcessorsPending: []string{"a"},
			}
			assert.Loosely(t, out, should.Resemble(expected))

			// Created instance and package entities.
			storedInst := &Instance{
				InstanceID: out.InstanceID,
				Package:    inst.Package,
			}
			storedPkg := &Package{Name: "a/b/c"}
			assert.Loosely(t, datastore.Get(ctx, storedInst, storedPkg), should.BeNil)

			assert.Loosely(t, storedInst, should.Resemble(expected))
			assert.Loosely(t, storedPkg, should.Resemble(pkg))

			assert.Loosely(t, GetEvents(ctx), should.Resemble([]*api.Event{
				{
					Kind:     api.EventKind_INSTANCE_CREATED,
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime.Add(1)),
					Package:  pkg.Name,
					Instance: expected.InstanceID,
				},
				{
					Kind:    api.EventKind_PACKAGE_CREATED,
					Who:     string(testutil.TestUser),
					When:    timestamppb.New(testutil.TestTime),
					Package: pkg.Name,
				},
			}))
		})

		t.Run("Existing package, new instance", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, pkg), should.BeNil)

			inst.RegisteredBy = "user:someoneelse@example.com"
			reg, out, err := RegisterInstance(ctx, inst, func(ctx context.Context, inst *Instance) error {
				inst.ProcessorsPending = []string{"a"}
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, reg, should.BeTrue)
			assert.Loosely(t, out, should.Resemble(&Instance{
				InstanceID:        inst.InstanceID,
				Package:           inst.Package,
				RegisteredBy:      inst.RegisteredBy,
				RegisteredTs:      inst.RegisteredTs,
				ProcessorsPending: []string{"a"},
			}))

			// Package entity wasn't touched.
			storedPkg := &Package{Name: "a/b/c"}
			assert.Loosely(t, datastore.Get(ctx, storedPkg), should.BeNil)
			assert.Loosely(t, storedPkg, should.Resemble(pkg))

			assert.Loosely(t, GetEvents(ctx), should.Resemble([]*api.Event{
				{
					Kind:     api.EventKind_INSTANCE_CREATED,
					Who:      string(testutil.TestUser),
					When:     timestamppb.New(testutil.TestTime),
					Package:  pkg.Name,
					Instance: inst.InstanceID,
				},
			}))
		})

		t.Run("Existing instance", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, pkg, inst), should.BeNil)

			modified := *inst
			modified.RegisteredBy = "user:someoneelse@example.com"
			reg, out, err := RegisterInstance(ctx, &modified, func(ctx context.Context, inst *Instance) error {
				panic("must not be called")
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, reg, should.BeFalse)
			assert.Loosely(t, out, should.Resemble(inst)) // the original one

			assert.Loosely(t, GetEvents(ctx), should.HaveLength(0))
		})
	})
}

func TestListInstances(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		inst := func(i int) *Instance {
			return &Instance{
				InstanceID:   fmt.Sprintf("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa%d", i),
				Package:      PackageKey(ctx, "a/b"),
				RegisteredTs: testutil.TestTime.Add(time.Duration(i) * time.Minute),
			}
		}
		for i := 0; i < 4; i++ {
			assert.Loosely(t, datastore.Put(ctx, inst(i)), should.BeNil)
		}

		t.Run("Full listing", func(t *ftt.Test) {
			out, cur, err := ListInstances(ctx, "a/b", 100, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cur, should.BeNil)
			assert.Loosely(t, out, should.Resemble([]*Instance{inst(3), inst(2), inst(1), inst(0)}))
		})

		t.Run("Paginated listing", func(t *ftt.Test) {
			out, cur, err := ListInstances(ctx, "a/b", 3, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cur, should.NotBeNil)
			assert.Loosely(t, out, should.Resemble([]*Instance{inst(3), inst(2), inst(1)}))

			out, cur, err = ListInstances(ctx, "a/b", 3, cur)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cur, should.BeNil)
			assert.Loosely(t, out, should.Resemble([]*Instance{inst(0)}))
		})
	})
}

func TestCheckInstance(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		put := func(pkg, iid string, failedProcs, pendingProcs []string) {
			assert.Loosely(t, datastore.Put(ctx,
				&Package{Name: pkg},
				&Instance{
					InstanceID:        iid,
					Package:           PackageKey(ctx, pkg),
					ProcessorsFailure: failedProcs,
					ProcessorsPending: pendingProcs,
				}), should.BeNil)
		}

		iid := common.ObjectRefToInstanceID(&api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA1,
			HexDigest: strings.Repeat("a", 40),
		})
		inst := &Instance{
			InstanceID: iid,
			Package:    PackageKey(ctx, "pkg"),
		}

		t.Run("Happy path", func(t *ftt.Test) {
			put("pkg", iid, nil, nil)
			assert.Loosely(t, CheckInstanceExists(ctx, inst), should.BeNil)
			assert.Loosely(t, CheckInstanceReady(ctx, inst), should.BeNil)
		})

		t.Run("No such instance", func(t *ftt.Test) {
			put("pkg", "f"+iid[1:], nil, nil)

			err := CheckInstanceExists(ctx, inst)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such instance"))

			err = CheckInstanceReady(ctx, inst)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such instance"))
		})

		t.Run("No such package", func(t *ftt.Test) {
			put("pkg2", iid, nil, nil)

			err := CheckInstanceExists(ctx, inst)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such package"))

			err = CheckInstanceReady(ctx, inst)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("no such package"))
		})

		t.Run("Failed processors", func(t *ftt.Test) {
			put("pkg", iid, []string{"f1", "f2"}, []string{"p1", "p2"})
			err := CheckInstanceReady(ctx, inst)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.Aborted))
			assert.Loosely(t, err, should.ErrLike("some processors failed to process this instance: f1, f2"))

			assert.Loosely(t, CheckInstanceExists(ctx, inst), should.BeNil) // doesn't care
		})

		t.Run("Pending processors", func(t *ftt.Test) {
			put("pkg", iid, nil, []string{"p1", "p2"})
			err := CheckInstanceReady(ctx, inst)
			assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.FailedPrecondition))
			assert.Loosely(t, err, should.ErrLike("the instance is not ready yet, pending processors: p1, p2"))

			assert.Loosely(t, CheckInstanceExists(ctx, inst), should.BeNil) // doesn't care
		})
	})
}

func TestFetchProcessors(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()
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
				assert.Loosely(t, p.WriteResult(map[string]string{"result": res}), should.BeNil)
			} else {
				p.Error = err
			}
			return p
		}

		assert.Loosely(t, datastore.Put(ctx,
			proc("f1", "", "fail 1"),
			proc("f2", "", "fail 2"),
			proc("s1", "success 1", ""),
			proc("s2", "success 2", ""),
		), should.BeNil)

		t.Run("Works", func(t *ftt.Test) {
			procs, err := FetchProcessors(ctx, inst)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, procs, should.Resemble([]*api.Processor{
				{
					Id:         "f1",
					State:      api.Processor_FAILED,
					FinishedTs: timestamppb.New(ts),
					Error:      "fail 1",
				},
				{
					Id:         "f2",
					State:      api.Processor_FAILED,
					FinishedTs: timestamppb.New(ts),
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
					FinishedTs: timestamppb.New(ts),
					Result: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"result": {Kind: &structpb.Value_StringValue{StringValue: "success 1"}},
						},
					},
				},
				{
					Id:         "s2",
					State:      api.Processor_SUCCEEDED,
					FinishedTs: timestamppb.New(ts),
					Result: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"result": {Kind: &structpb.Value_StringValue{StringValue: "success 2"}},
						},
					},
				},
			}))
		})
	})
}
