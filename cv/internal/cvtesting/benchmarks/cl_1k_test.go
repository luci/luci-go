// Copyright 2021 The LUCI Authors.
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

package benchmarks

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

var epoch = testclock.TestRecentTimeUTC

// clidOffset emulates realistic datastore-generated CLIDs.
const clidOffset = 4683683202072576

func benchmarkRAMper1KObjects(b *testing.B, objectKind string, work func()) (before, after runtime.MemStats) {
	runtime.GC()
	runtime.ReadMemStats(&before)

	b.ReportAllocs()
	work()

	runtime.ReadMemStats(&after)
	obj1 := float64(after.HeapAlloc-before.HeapAlloc) / 1024 / 1024 / float64(b.N)
	b.ReportMetric(obj1*1000, "MB-heap/1K-"+objectKind)
	return
}

// BenchmarkLoadCLsUsage1K estimates how much RAM keeping 1K CL objects will take.
func BenchmarkLoadCLsUsage1K(b *testing.B) {
	if b.N > 100000 {
		b.Skip("Specify lower `-benchtime=XX`")
	}
	ctx := putCLs(b.N)
	cls := initCLObjects(b.N)

	benchmarkRAMper1KObjects(b, "CL", func() {
		if err := datastore.Get(ctx, cls); err != nil {
			panic(err)
		}
	})
}

// BenchmarkPMStatePCLRAMUsage1K estimates how much RAM keeping 1K PM.PState.PCL
// objects will take.
func BenchmarkPMStatePCLRAMUsage1K(b *testing.B) {
	if b.N > 100000 {
		b.Skip("Specify lower `-benchtime=XX`")
	}
	cis := makeCIs(b.N)
	cls := make([]*changelist.CL, b.N)
	for i, ci := range cis {
		cls[i] = makeCL(ci)
	}

	out := make([]*prjpb.PCL, b.N)

	benchmarkRAMper1KObjects(b, "prjpb.PCL", func() {
		for i, cl := range cls {
			out[i] = makePCL(cl)
		}
	})

	bs, _ := proto.Marshal(&prjpb.PState{Pcls: out})
	v := float64(len(bs)) / 1024 / 1024 / float64(b.N)
	b.ReportMetric(v*1000, "MB-pb/1K-"+"prjpb.PCL")

	var z zstd.Encoder
	compressed := z.EncodeAll(bs, nil)
	v = float64(len(compressed)) / 1024 / 1024 / float64(b.N)
	b.ReportMetric(v*1000, "MB-pb-zstd/1K-"+"prjpb.PCL")
}

func putCLs(N int) context.Context {
	ctx := memory.Use(context.Background())
	cis := makeCIs(N)
	for _, ci := range cis {
		cl := makeCL(ci)
		if err := datastore.Put(ctx, cl); err != nil {
			panic(err)
		}
	}
	return ctx
}

func initCLObjects(N int) []*changelist.CL {
	out := make([]*changelist.CL, N)
	for i := range out {
		out[i] = &changelist.CL{ID: common.CLID(i + 1 + clidOffset)}
	}
	return out
}

func makePCL(cl *changelist.CL) *prjpb.PCL {
	// Copy deps for realism.
	deps := make([]*changelist.Dep, len(cl.Snapshot.GetDeps()))
	copy(deps, cl.Snapshot.GetDeps())

	trs := trigger.Find(&trigger.FindInput{
		ChangeInfo:  cl.Snapshot.GetGerrit().GetInfo(),
		ConfigGroup: &cfgpb.ConfigGroup{}})
	// Copy trigger email string for realism of allocations.
	tr := trs.GetCqVoteTrigger()
	tr.Email = tr.Email + " "
	return &prjpb.PCL{
		Clid:               int64(cl.ID),
		Eversion:           cl.EVersion,
		Status:             prjpb.PCL_OK,
		Submitted:          false,
		ConfigGroupIndexes: []int32{0},
		Deps:               deps,
		Triggers:           trs,
	}
}

func makeCL(ci *gerritpb.ChangeInfo) *changelist.CL {
	now := epoch.Add(time.Hour * 1000)
	mps, ps, err := gerrit.EquivalentPatchsetRange(ci)
	if err != nil {
		panic(err)
	}
	num := clidOffset + ci.GetNumber()
	// The deps aren't supposed to make sense, only to resemble what actual
	// CL stores.
	return &changelist.CL{
		ID:         common.CLID(num),
		ExternalID: changelist.MustGobID("host", ci.GetNumber()),
		Snapshot: &changelist.Snapshot{
			Kind: &changelist.Snapshot_Gerrit{Gerrit: &changelist.Gerrit{
				Info:  ci,
				Files: []string{"a.cpp"},
				Host:  "host",
				GitDeps: []*changelist.GerritGitDep{
					{Change: num - 1, Immediate: true},
					{Change: num - 2, Immediate: false},
				},
				SoftDeps: []*changelist.GerritSoftDep{
					{Change: num + 1, Host: "host-2"},
				},
			}},
			Deps: []*changelist.Dep{
				{Clid: num - 2, Kind: changelist.DepKind_SOFT},
				{Clid: num - 1, Kind: changelist.DepKind_HARD},
				{Clid: num + 1, Kind: changelist.DepKind_SOFT}, // num:host-2
			},
			ExternalUpdateTime:    ci.GetUpdated(),
			LuciProject:           "test",
			MinEquivalentPatchset: int32(mps),
			Patchset:              int32(ps),
		},
		ApplicableConfig: &changelist.ApplicableConfig{
			Projects: []*changelist.ApplicableConfig_Project{
				{Name: "test", ConfigGroupIds: []string{"main"}},
			},
		},
		Access: &changelist.Access{
			ByProject: map[string]*changelist.Access_Project{
				"test-2": {
					NoAccess:     true,
					NoAccessTime: timestamppb.New(now),
					UpdateTime:   timestamppb.New(now),
				},
			},
		},
		EVersion:       1,
		UpdateTime:     now,
		IncompleteRuns: common.MakeRunIDs("test/run-001"),
	}
}

func makeCIs(N int) []*gerritpb.ChangeInfo {
	cis := make([]*gerritpb.ChangeInfo, N)
	for i := range cis {
		cis[i] = makeCI(i)
		changelist.RemoveUnusedGerritInfo(cis[i])
	}
	return cis
}

func makeCI(i int) *gerritpb.ChangeInfo {
	di := time.Duration(i)
	u1 := fmt.Sprintf("user-%d", 2*i+1)
	u2 := fmt.Sprintf("user-%d", 2*i+2)
	return gf.CI(
		i+1,
		gf.Owner("owner-3"),
		gf.PS(5),
		gf.AllRevs(),
		gf.CQ(+1, epoch.Add(di*time.Minute), u1),
		gf.Vote("Code-Review", +2, epoch.Add(di*time.Minute), u2),
		gf.Messages(
			&gerritpb.ChangeMessageInfo{
				Author:  gf.U(u1),
				Date:    timestamppb.New(epoch.Add(di * time.Minute)),
				Message: "this is message",
				Id:      "sha-something-1",
			},
			&gerritpb.ChangeMessageInfo{
				Author:  gf.U(u2),
				Date:    timestamppb.New(epoch.Add(di * time.Minute)),
				Message: "this is a better message",
				Id:      "sha-something-2",
			},
		),
	)
}
