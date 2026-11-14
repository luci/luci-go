// Copyright 2026 The LUCI Authors.
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

package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	config "go.chromium.org/luci/gce/api/config/v1"
	tasks "go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

func baseReimageTestConfig() *model.Config {
	return &model.Config{
		ID: "cfg-1",
		Config: &config.Config{
			Prefix:          "chrome-jammy",
			Swarming:        "https://chromium-swarm-dev.appspot.com",
			CurrentAmount:   2,
			RecycleStrategy: config.RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE,
			Attributes: &config.VM{
				Project:       "proj-1",
				Zone:          "us-central1-c",
				FallbackZones: []string{"us-central1-a"},
				MachineType:   "zones/{{.Zone}}/machineTypes/e2-standard-4",
				Disk: []*config.Disk{
					{
						Image: "projects/img-proj/global/images/jammy-v2",
						Size:  100,
						Type:  "zones/{{.Zone}}/diskTypes/pd-ssd",
					},
				},
				Label: map[string]string{"resource_group": "chrome"},
			},
		},
	}
}

func baseReimageTestVM() *model.VM {
	attrs := &config.VM{
		Project:       "proj-1",
		Zone:          "us-central1-c",
		FallbackZones: []string{"us-central1-a"},
		MachineType:   "zones/{{.Zone}}/machineTypes/e2-standard-4",
		Disk: []*config.Disk{
			{
				Image: "projects/img-proj/global/images/jammy-v1",
				Size:  100,
				Type:  "zones/{{.Zone}}/diskTypes/pd-ssd",
			},
		},
		Label: map[string]string{"resource_group": "chrome"},
	}
	attrs.SetZone("us-central1-a")
	return &model.VM{
		ID:         "chrome-jammy-0",
		Index:      0,
		Config:     "cfg-1",
		Prefix:     "chrome-jammy",
		Hostname:   "chrome-jammy-0-abcd",
		Swarming:   "https://chromium-swarm-dev.appspot.com",
		URL:        "https://www.googleapis.com/compute/v1/projects/proj-1/zones/us-central1-a/instances/chrome-jammy-0-abcd",
		Created:    1000,
		Connected:  1010,
		Attributes: *attrs,
	}
}

func reimageTaskForVM(vm *model.VM) *tasks.ReimageInstance {
	return &tasks.ReimageInstance{
		Id:      vm.ID,
		Url:     vm.URL,
		Created: vm.Created,
	}
}

func reimageMetricCount(c context.Context, status, reason string) int64 {
	for _, cell := range tsmon.Store(c).GetAll(c) {
		if cell.MetricInfo.Name == "gce/instances/reimages" &&
			len(cell.CellData.FieldVals) == 5 &&
			cell.CellData.FieldVals[3] == status &&
			cell.CellData.FieldVals[4] == reason {
			return cell.CellData.Value.(int64)
		}
	}
	return 0
}

func TestRecycleInstanceDecision(t *testing.T) {
	t.Parallel()

	ftt.Run("recycleInstanceAsync", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		c, _ := tsmon.WithDummyInMemory(memory.Use(t.Context()))
		c, sched := tq.TestingContext(c, dsp)
		c = withDispatcher(c, dsp)

		cfg := baseReimageTestConfig()
		vm := baseReimageTestVM()

		t.Run("compatible image uprev in fallback zone schedules ReimageInstance", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, recycleInstanceAsync(c, vm), should.BeNil)
			assert.Loosely(t, sched.Tasks(), should.HaveLength(1))
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.Match(&tasks.ReimageInstance{
				Id:      vm.ID,
				Url:     vm.URL,
				Created: vm.Created,
			}))

			gotCfg, gotTarget, reason, err := reimageConfig(c, vm)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, reason, should.BeEmpty)
			assert.Loosely(t, gotCfg, should.NotBeNil)
			assert.Loosely(t, gotTarget.GetZone(), should.Equal("us-central1-a"))
			assert.Loosely(t, gotTarget.GetMachineType(), should.Equal("zones/us-central1-a/machineTypes/e2-standard-4"))
			assert.Loosely(t, gotTarget.GetDisk()[0].GetImage(), should.Equal("projects/img-proj/global/images/jammy-v2"))
			assert.Loosely(t, gotTarget.GetDisk()[0].GetType(), should.Equal("zones/us-central1-a/diskTypes/pd-ssd"))
		})

		t.Run("delete and recreate strategy destroys without skipped metric", func(t *ftt.Test) {
			cfg.Config.RecycleStrategy = config.RecycleStrategy_RECYCLE_STRATEGY_DELETE_AND_RECREATE
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, recycleInstanceAsync(c, vm), should.BeNil)
			assert.Loosely(t, sched.Tasks(), should.HaveLength(1))
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "skipped", "drained"), should.Equal(int64(0)))
		})

		t.Run("deleted config destroys without skipped metric", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, vm), should.BeNil)
			assert.Loosely(t, recycleInstanceAsync(c, vm), should.BeNil)
			assert.Loosely(t, sched.Tasks(), should.HaveLength(1))
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
		})

		t.Run("drained or scaled-down VM destroys with drained metric", func(t *ftt.Test) {
			cfg.Config.CurrentAmount = 0
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, recycleInstanceAsync(c, vm), should.BeNil)
			assert.Loosely(t, sched.Tasks(), should.HaveLength(1))
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "skipped", "drained"), should.Equal(int64(1)))
		})

		t.Run("swarming server changed destroys with swarming_changed metric", func(t *ftt.Test) {
			cfg.Config.Swarming = "https://other-swarm.appspot.com"
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, recycleInstanceAsync(c, vm), should.BeNil)
			assert.Loosely(t, sched.Tasks(), should.HaveLength(1))
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "skipped", "swarming_changed"), should.Equal(int64(1)))
		})

		t.Run("active fallback zone removed destroys with zone_changed metric", func(t *ftt.Test) {
			cfg.Config.Attributes.FallbackZones = []string{"us-central1-b"}
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, recycleInstanceAsync(c, vm), should.BeNil)
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "skipped", "zone_changed"), should.Equal(int64(1)))
		})

		t.Run("disk size changed destroys with disks_changed metric", func(t *ftt.Test) {
			cfg.Config.Attributes.Disk[0].Size = 200
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, recycleInstanceAsync(c, vm), should.BeNil)
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "skipped", "disks_changed"), should.Equal(int64(1)))
		})

		t.Run("machine type changed destroys with machine_type_changed metric", func(t *ftt.Test) {
			cfg.Config.Attributes.MachineType = "zones/{{.Zone}}/machineTypes/e2-standard-8"
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, recycleInstanceAsync(c, vm), should.BeNil)
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "skipped", "machine_type_changed"), should.Equal(int64(1)))
		})

		t.Run("metadata changed destroys with attributes_changed metric", func(t *ftt.Test) {
			cloned := proto.Clone(cfg.Config.Attributes).(*config.VM)
			cloned.Tag = []string{"new-tag"}
			cfg.Config.Attributes = cloned
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, recycleInstanceAsync(c, vm), should.BeNil)
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "skipped", "attributes_changed"), should.Equal(int64(1)))
		})
	})

	ftt.Run("reimageInstance invalid payloads", t, func(t *ftt.Test) {
		c := memory.Use(t.Context())
		assert.Loosely(t, reimageInstance(c, nil), should.ErrLike("unexpected payload"))
		assert.Loosely(t, reimageInstance(c, &tasks.ReimageInstance{}), should.ErrLike("ID is required"))
		assert.Loosely(t, reimageInstance(c, &tasks.ReimageInstance{Id: "id"}), should.ErrLike("URL is required"))
	})
}

type funcRoundTripper func(*http.Request) (*http.Response, error)

func (f funcRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestSendReimage(t *testing.T) {
	t.Parallel()

	ftt.Run("sendReimage and error handling", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)

		now := time.Date(2026, 11, 4, 1, 0, 0, 0, time.UTC)
		c, tc := testclock.UseTime(memory.Use(t.Context()), now)
		c, _ = tsmon.WithDummyInMemory(c)
		c, sched := tq.TestingContext(c, dsp)

		var gotUpdateReq *http.Request
		var gotUpdateInst compute.Instance
		var putCalls int
		var decodeErr error
		getStatus := http.StatusOK
		getReason := "errReason"
		updateStatus := http.StatusOK
		updateReason := "errReason"
		updateOp := &compute.Operation{Name: "op-reimage-1", Status: "RUNNING"}
		fetchedDisks := []*compute.AttachedDisk{
			{
				AutoDelete: true,
				Boot:       true,
				DeviceName: "persistent-disk-0",
				Interface:  "SCSI",
				Mode:       "READ_WRITE",
				Type:       "PERSISTENT",
				Source:     "projects/proj-1/zones/us-central1-a/disks/chrome-jammy-0-abcd",
			},
		}

		rt := funcRoundTripper(func(req *http.Request) (*http.Response, error) {
			switch req.Method {
			case http.MethodGet:
				if getStatus != http.StatusOK {
					return gceErrorHTTPResponse(getStatus, getReason, "get failed"), nil
				}
				return jsonHTTPResponse(http.StatusOK, &compute.Instance{
					Name:             "chrome-jammy-0-abcd",
					Fingerprint:      "inst-fp-1",
					LabelFingerprint: "lbl-fp-1",
					Disks:            fetchedDisks,
				}), nil
			case http.MethodPut:
				putCalls++
				gotUpdateReq = req
				if req.Body != nil {
					defer req.Body.Close()
					decodeErr = json.NewDecoder(req.Body).Decode(&gotUpdateInst)
				}
				if updateStatus != http.StatusOK {
					return gceErrorHTTPResponse(updateStatus, updateReason, "update failed"), nil
				}
				return jsonHTTPResponse(http.StatusOK, updateOp), nil
			default:
				return jsonHTTPResponse(http.StatusMethodNotAllowed, nil), nil
			}
		})
		gceSrv, err := compute.New(&http.Client{Transport: rt})
		assert.Loosely(t, err, should.BeNil)
		c = withCompute(withDispatcher(c, dsp), ComputeService{Stable: gceSrv})

		cfg := baseReimageTestConfig()
		cfg.Config.Lifetime = &config.TimePeriod{Time: &config.TimePeriod_Duration{Duration: "30m"}}
		cfg.Config.Timeout = &config.TimePeriod{Time: &config.TimePeriod_Seconds{Seconds: 600}}
		cfg.Config.Revision = "rev-2"
		vm := baseReimageTestVM()

		t.Run("sends instances.update with RESTART, fingerprints, and RECREATE_DISK", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, reimageInstance(c, reimageTaskForVM(vm)), should.BeNil)
			assert.Loosely(t, decodeErr, should.BeNil)
			assert.Loosely(t, gotUpdateReq, should.NotBeNil)
			assert.Loosely(t, gotUpdateReq.URL.Query().Get("mostDisruptiveAllowedAction"), should.Equal("RESTART"))
			wantRID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("reimage-%s-%d", vm.URL, vm.Created))).String()
			assert.Loosely(t, gotUpdateReq.URL.Query().Get("requestId"), should.Equal(wantRID))
			assert.Loosely(t, gotUpdateInst.Fingerprint, should.Equal("inst-fp-1"))
			assert.Loosely(t, gotUpdateInst.LabelFingerprint, should.Equal("lbl-fp-1"))
			assert.Loosely(t, gotUpdateInst.Disks, should.HaveLength(1))
			assert.Loosely(t, gotUpdateInst.Disks[0].Source, should.BeEmpty)
			assert.Loosely(t, gotUpdateInst.Disks[0].InitializeParams.OnUpdateAction, should.Equal("RECREATE_DISK"))
			assert.Loosely(t, gotUpdateInst.Disks[0].InitializeParams.SourceImage, should.Equal("projects/img-proj/global/images/jammy-v2"))

			stored := &model.VM{ID: vm.ID}
			assert.Loosely(t, datastore.Get(c, stored), should.BeNil)
			assert.Loosely(t, stored.Reimaging, should.Equal(now.Unix()))
			assert.Loosely(t, stored.ReimageOp, should.Equal("op-reimage-1"))
			assert.Loosely(t, stored.Revision, should.Equal("rev-2"))
			assert.Loosely(t, stored.Timeout, should.Equal(int64(600)))
			assert.Loosely(t, stored.Attributes.GetDisk()[0].GetImage(), should.Equal("projects/img-proj/global/images/jammy-v2"))
		})

		t.Run("ignores stale Created or mismatched URL tasks without calling GCE", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, reimageInstance(c, &tasks.ReimageInstance{
				Id:      vm.ID,
				Url:     vm.URL,
				Created: vm.Created - 500,
			}), should.BeNil)
			assert.Loosely(t, reimageInstance(c, &tasks.ReimageInstance{
				Id:      vm.ID,
				Url:     vm.URL + "-other",
				Created: vm.Created,
			}), should.BeNil)
			assert.Loosely(t, putCalls, should.BeZero)
			assert.Loosely(t, sched.Tasks(), should.BeEmpty)
		})

		t.Run("transient 503, 412 precondition, and 403 rateLimitExceeded retry before 20m timeout", func(t *ftt.Test) {
			updateStatus = http.StatusServiceUnavailable
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, reimageInstance(c, reimageTaskForVM(vm)), should.ErrLike("503"))
			assert.Loosely(t, sched.Tasks(), should.BeEmpty)

			// HTTP 412 fingerprint conflict and GCE 403 rateLimitExceeded also retry.
			updateStatus = http.StatusPreconditionFailed
			assert.Loosely(t, reimageInstance(c, reimageTaskForVM(vm)), should.ErrLike("412"))
			assert.Loosely(t, sched.Tasks(), should.BeEmpty)

			updateStatus = http.StatusForbidden
			updateReason = "rateLimitExceeded"
			assert.Loosely(t, reimageInstance(c, reimageTaskForVM(vm)), should.ErrLike("403"))
			assert.Loosely(t, sched.Tasks(), should.BeEmpty)

			stored := &model.VM{ID: vm.ID}
			assert.Loosely(t, datastore.Get(c, stored), should.BeNil)
			assert.Loosely(t, stored.Reimaging, should.Equal(now.Unix()))
			assert.Loosely(t, stored.ReimageOp, should.BeEmpty)

			// Exceeding maxReimageDuration latches Drained and destroys the VM.
			tc.Add(maxReimageDuration + time.Second)
			assert.Loosely(t, reimageInstance(c, reimageTaskForVM(vm)), should.BeNil)
			assert.Loosely(t, sched.Tasks(), should.HaveLength(1))
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "abandoned", "timeout"), should.Equal(int64(1)))
			assert.Loosely(t, datastore.Get(c, stored), should.BeNil)
			assert.Loosely(t, stored.Drained, should.BeTrue)
			assert.Loosely(t, stored.Reimaging, should.Equal(int64(0)))
		})

		t.Run("permanent 400 and non-rate-limit 403 abandon with Drained latched", func(t *ftt.Test) {
			updateStatus = http.StatusForbidden
			updateReason = "forbidden"
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, reimageInstance(c, reimageTaskForVM(vm)), should.BeNil)
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "abandoned", "api_error"), should.Equal(int64(1)))

			stored := &model.VM{ID: vm.ID}
			assert.Loosely(t, datastore.Get(c, stored), should.BeNil)
			assert.Loosely(t, stored.Drained, should.BeTrue)
		})

		t.Run("404 instance not found abandons with not_found metric", func(t *ftt.Test) {
			getStatus = http.StatusNotFound
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, reimageInstance(c, reimageTaskForVM(vm)), should.BeNil)
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "abandoned", "not_found"), should.Equal(int64(1)))
		})

		t.Run("scratch disk mismatch abandons and destroys", func(t *ftt.Test) {
			fetchedDisks[0].Type = "SCRATCH"
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, reimageInstance(c, reimageTaskForVM(vm)), should.BeNil)
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "abandoned", "disk_mismatch"), should.Equal(int64(1)))
		})

		t.Run("operation error abandons and destroys", func(t *ftt.Test) {
			updateOp = &compute.Operation{
				Name:   "op-err",
				Status: "DONE",
				Error: &compute.OperationError{
					Errors: []*compute.OperationErrorErrors{{Code: "QUOTA_EXCEEDED"}},
				},
			}
			assert.Loosely(t, datastore.Put(c, cfg, vm), should.BeNil)
			assert.Loosely(t, reimageInstance(c, reimageTaskForVM(vm)), should.BeNil)
			assert.Loosely(t, sched.Tasks().Payloads()[0], should.HaveType[*tasks.DestroyInstance])
			assert.Loosely(t, reimageMetricCount(c, "abandoned", "operation_error"), should.Equal(int64(1)))
		})
	})
}
