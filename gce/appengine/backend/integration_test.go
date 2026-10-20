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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/backend/internal/metrics"
	"go.chromium.org/luci/gce/appengine/model"
	"go.chromium.org/luci/gce/appengine/rpc"
)

// gceCall records a single HTTP request received by fakeGCEServer.
type gceCall struct {
	Method      string
	Project     string
	Zone        string
	Name        string
	RequestID   string
	MachineType string
	DiskType    string
}

// gceInsertOutcome defines a scripted response for an Instances.Insert call in
// a specific zone.
type gceInsertOutcome struct {
	httpStatus int
	errReason  string
	errMessage string
	opStatus   string
	opErrCode  string
	opErrMsg   string
}

// gceDone returns a scripted outcome where Instances.Insert succeeds
// immediately with an HTTP 200 DONE operation.
func gceDone() gceInsertOutcome {
	return gceInsertOutcome{
		httpStatus: http.StatusOK,
		opStatus:   "DONE",
	}
}

// gceRunning returns a scripted outcome where Instances.Insert starts an
// in-flight asynchronous operation with HTTP 200 RUNNING status.
func gceRunning() gceInsertOutcome {
	return gceInsertOutcome{
		httpStatus: http.StatusOK,
		opStatus:   "RUNNING",
	}
}

// gceOpError returns a scripted outcome where Instances.Insert returns an HTTP
// 200 DONE operation containing a GCE operation error (such as
// ZONE_RESOURCE_POOL_EXHAUSTED or QUOTA_EXCEEDED).
func gceOpError(code, msg string) gceInsertOutcome {
	return gceInsertOutcome{
		httpStatus: http.StatusOK,
		opStatus:   "DONE",
		opErrCode:  code,
		opErrMsg:   msg,
	}
}

// gceHTTPError returns a scripted outcome where Instances.Insert fails at the
// HTTP layer with a googleapi.Error status, reason, and message.
func gceHTTPError(status int, reason, msg string) gceInsertOutcome {
	return gceInsertOutcome{
		httpStatus: status,
		errReason:  reason,
		errMessage: msg,
	}
}

// storedGCEInstance tracks a simulated GCE VM instance and the requestId that
// created it in a specific zone.
type storedGCEInstance struct {
	instance  *compute.Instance
	requestID string
}

// fakeGCEServer is a stateful http.RoundTripper that mimics the Google Compute
// Engine v1 REST API for end-to-end VM creation integration tests. It tracks
// created instances per zone, enforces requestId idempotency (returning HTTP
// 409 alreadyExists when a different requestId attempts to insert an existing
// instance name in the same zone), and replays scripted per-zone responses.
type fakeGCEServer struct {
	mu          sync.Mutex
	calls       []gceCall
	zoneScripts map[string][]gceInsertOutcome
	instances   map[string]map[string]*storedGCEInstance
}

// newFakeGCEServer creates an initialized fakeGCEServer.
func newFakeGCEServer() *fakeGCEServer {
	return &fakeGCEServer{
		zoneScripts: make(map[string][]gceInsertOutcome),
		instances:   make(map[string]map[string]*storedGCEInstance),
	}
}

// scriptZone configures a sequence of Insert outcomes for the given zone. When
// the sequence is exhausted, subsequent Insert calls in that zone default to
// gceDone().
func (f *fakeGCEServer) scriptZone(zone string, outcomes ...gceInsertOutcome) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.zoneScripts[zone] = append([]gceInsertOutcome(nil), outcomes...)
}

// nextOutcomeLocked pops and returns the next scripted outcome for the given
// zone, defaulting to gceDone() when no scripted outcomes remain.
func (f *fakeGCEServer) nextOutcomeLocked(zone string) gceInsertOutcome {
	queue := f.zoneScripts[zone]
	if len(queue) == 0 {
		return gceDone()
	}
	out := queue[0]
	f.zoneScripts[zone] = queue[1:]
	return out
}

// RoundTrip implements http.RoundTripper to intercept and serve Google Compute
// Engine v1 Instances.Insert (POST), Instances.Get (GET), and Instances.Delete
// (DELETE) requests.
func (f *fakeGCEServer) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Expected paths:
	// POST   .../projects/{project}/zones/{zone}/instances
	// GET    .../projects/{project}/zones/{zone}/instances/{name}
	// DELETE .../projects/{project}/zones/{zone}/instances/{name}
	parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	projIdx := -1
	for i, p := range parts {
		if p == "projects" {
			projIdx = i
			break
		}
	}
	if projIdx == -1 || len(parts) < projIdx+5 || parts[projIdx+2] != "zones" || parts[projIdx+4] != "instances" {
		return jsonHTTPResponse(http.StatusNotFound, map[string]any{"error": "unrecognized path: " + req.URL.Path}), nil
	}
	project := parts[projIdx+1]
	zone := parts[projIdx+3]
	var instanceName string
	if len(parts) > projIdx+5 {
		instanceName = parts[projIdx+5]
	}
	reqID := req.URL.Query().Get("requestId")

	switch req.Method {
	case http.MethodPost:
		var inst compute.Instance
		if req.Body != nil {
			defer req.Body.Close()
			if err := json.NewDecoder(req.Body).Decode(&inst); err != nil {
				return nil, err
			}
		}
		var diskType string
		if len(inst.Disks) > 0 && inst.Disks[0].InitializeParams != nil {
			diskType = inst.Disks[0].InitializeParams.DiskType
		}
		f.calls = append(f.calls, gceCall{
			Method:      http.MethodPost,
			Project:     project,
			Zone:        zone,
			Name:        inst.Name,
			RequestID:   reqID,
			MachineType: inst.MachineType,
			DiskType:    diskType,
		})

		if f.instances[zone] == nil {
			f.instances[zone] = make(map[string]*storedGCEInstance)
		}
		// Enforce GCE requestId idempotency vs. HTTP 409 alreadyExists conflict:
		// if an instance with the same name already exists in this zone from a
		// different requestId, GCE rejects the new Insert with 409 alreadyExists.
		if existing, ok := f.instances[zone][inst.Name]; ok && existing.requestID != reqID {
			return gceErrorHTTPResponse(
				http.StatusConflict,
				"alreadyExists",
				fmt.Sprintf("The resource 'projects/%s/zones/%s/instances/%s' already exists", project, zone, inst.Name),
			), nil
		}

		outcome := f.nextOutcomeLocked(zone)
		if outcome.httpStatus != http.StatusOK {
			return gceErrorHTTPResponse(outcome.httpStatus, outcome.errReason, outcome.errMessage), nil
		}
		if outcome.opErrCode != "" {
			// If an in-flight async operation failed with an operation error (such as
			// ZONE_RESOURCE_POOL_EXHAUSTED), the instance does not exist in this zone.
			delete(f.instances[zone], inst.Name)
			return jsonHTTPResponse(http.StatusOK, &compute.Operation{
				Status: "DONE",
				Error: &compute.OperationError{
					Errors: []*compute.OperationErrorErrors{
						{
							Code:    outcome.opErrCode,
							Message: outcome.opErrMsg,
						},
					},
				},
			}), nil
		}

		selfLink := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s", project, zone, inst.Name)
		f.instances[zone][inst.Name] = &storedGCEInstance{
			requestID: reqID,
			instance: &compute.Instance{
				Name:              inst.Name,
				MachineType:       inst.MachineType,
				CreationTimestamp: "2026-10-20T00:00:00.000-07:00",
				SelfLink:          selfLink,
				NetworkInterfaces: []*compute.NetworkInterface{
					{
						NetworkIP: "10.0.0.10",
						AccessConfigs: []*compute.AccessConfig{
							{NatIP: "34.100.0.10"},
						},
					},
				},
			},
		}
		return jsonHTTPResponse(http.StatusOK, &compute.Operation{
			Status:     outcome.opStatus,
			EndTime:    "2026-10-20T00:00:00.000-07:00",
			TargetLink: selfLink,
		}), nil

	case http.MethodGet:
		f.calls = append(f.calls, gceCall{
			Method:  http.MethodGet,
			Project: project,
			Zone:    zone,
			Name:    instanceName,
		})
		stored := f.instances[zone][instanceName]
		if stored == nil {
			return gceErrorHTTPResponse(
				http.StatusNotFound,
				"notFound",
				fmt.Sprintf("The resource 'projects/%s/zones/%s/instances/%s' was not found", project, zone, instanceName),
			), nil
		}
		return jsonHTTPResponse(http.StatusOK, stored.instance), nil

	case http.MethodDelete:
		f.calls = append(f.calls, gceCall{
			Method:    http.MethodDelete,
			Project:   project,
			Zone:      zone,
			Name:      instanceName,
			RequestID: reqID,
		})
		if f.instances[zone] != nil {
			delete(f.instances[zone], instanceName)
		}
		return jsonHTTPResponse(http.StatusOK, &compute.Operation{
			Status: "DONE",
		}), nil

	default:
		return jsonHTTPResponse(http.StatusMethodNotAllowed, nil), nil
	}
}

// jsonHTTPResponse serializes body as JSON and returns an *http.Response with
// the given HTTP status code.
func jsonHTTPResponse(status int, body any) *http.Response {
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	hdr := make(http.Header)
	hdr.Set("Content-Type", "application/json")
	return &http.Response{
		StatusCode: status,
		Header:     hdr,
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
}

// gceErrorHTTPResponse constructs a Google Compute Engine JSON error response
// matching the wire format parsed by googleapi.CheckResponse.
func gceErrorHTTPResponse(status int, reason, message string) *http.Response {
	payload := map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": message,
			"errors": []map[string]string{
				{
					"reason":  reason,
					"message": message,
				},
			},
		},
	}
	return jsonHTTPResponse(status, payload)
}

// TestVMCreationIntegration verifies the end-to-end VM creation pipeline in
// gce-provider against a stateful fake Google Compute Engine REST API server
// (fakeGCEServer). Each subtest exercises the full lifecycle from the moment a
// VM configuration is received via RPC (rpc.Config.Ensure / rpc.Config.Update),
// through the cron and task queue stages (expandConfigsAsync -> expandConfig ->
// createVM -> createInstancesAsync -> createInstance -> countVMsAsync ->
// countVMs), covering:
//  1. Standard single-zone VM creation for both static amount-based pools and
//     dynamic CloudBots DUT-backed pools.
//  2. Multi-zone fallback configs (fallback_zones) succeeding on the root
//     (primary) zone when capacity is available.
//  3. Multi-zone fallback configs rotating sequentially across candidate zones
//     when the root and intermediate zones fail with synchronous or
//     asynchronous GCE capacity stockouts.
//  4. Transient HTTP 503 backendError responses while polling an in-flight
//     RUNNING operation in a fallback zone, verifying that gce-provider retries
//     the same zone with the same requestId instead of rotating zones and
//     triggering an HTTP 409 alreadyExists conflict.
//  5. Multi-tick asynchronous RUNNING operations across candidate zones that
//     exhaust all fallback zones and wrap back around to the root zone.
//  6. Permanent non-stockout GCE operation errors (such as QUOTA_EXCEEDED),
//     verifying that gce-provider deletes the unassigned VM entity without
//     rotating across fallback zones and recreates it on the next expand cycle.
func TestVMCreationIntegration(t *testing.T) {
	t.Parallel()

	ftt.Run("end-to-end VM creation from config ingestion to GCE instance", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		fakeGCE := newFakeGCEServer()
		gceSrv, err := compute.New(&http.Client{Transport: fakeGCE})
		assert.Loosely(t, err, should.BeNil)

		c, _ := testclock.UseTime(memory.Use(context.Background()), testclock.TestTimeUTC)
		c = mathrand.Set(c, rand.New(rand.NewSource(1)))
		c, _ = tsmon.WithDummyInMemory(c)
		c = auth.WithState(c, &authtest.FakeState{
			Identity:       "user:owner@example.com",
			IdentityGroups: []string{"owners"},
		})
		c, sched := tq.TestingContext(c, dsp)
		c = withCompute(withDispatcher(c, dsp), ComputeService{Stable: gceSrv})
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)

		cfgServer := &rpc.Config{}

		// resetScheduler clears previously queued tasks from the in-memory task
		// queue scheduler before running the next pipeline stage.
		resetScheduler := func() {
			c, sched = tq.TestingContext(c, dsp)
			c = withCompute(withDispatcher(c, dsp), ComputeService{Stable: gceSrv})
		}

		// receiveConfig simulates gce-provider receiving a VM config (and optional
		// CloudBots DUT list update) and storing it in Datastore.
		receiveConfig := func(cfg *config.Config) {
			duts := cfg.GetDuts()
			_, err := cfgServer.Ensure(c, &config.EnsureRequest{
				Id:     cfg.GetPrefix(),
				Config: cfg,
			})
			assert.Loosely(t, err, should.BeNil)
			if len(duts) > 0 {
				_, err := cfgServer.Update(c, &config.UpdateRequest{
					Id: cfg.GetPrefix(),
					Config: &config.Config{
						Duts: duts,
					},
					UpdateMask: &fieldmaskpb.FieldMask{
						Paths: []string{"config.duts"},
					},
				})
				assert.Loosely(t, err, should.BeNil)
			}
		}

		// runExpandAndCreateVMs runs the expand-configs cron and executes all
		// resulting expand-config and create-vm tasks in the task queue.
		runExpandAndCreateVMs := func() {
			resetScheduler()
			assert.Loosely(t, expandConfigsAsync(c), should.BeNil)
			expandTasks := sched.Tasks().Payloads()
			resetScheduler()
			for _, payload := range expandTasks {
				if _, ok := payload.(*tasks.ExpandConfig); ok {
					assert.Loosely(t, expandConfig(c, payload), should.BeNil)
				}
			}
			createVMTasks := sched.Tasks().Payloads()
			resetScheduler()
			for _, payload := range createVMTasks {
				if _, ok := payload.(*tasks.CreateVM); ok {
					assert.Loosely(t, createVM(c, payload), should.BeNil)
				}
			}
		}

		// runCreateInstancesTick runs one tick of the create-instances cron and
		// executes each scheduled CreateInstance task once.
		runCreateInstancesTick := func() map[string]error {
			resetScheduler()
			assert.Loosely(t, createInstancesAsync(c), should.BeNil)
			pending := sched.Tasks().Payloads()
			resetScheduler()
			errs := make(map[string]error, len(pending))
			for _, payload := range pending {
				if tk, ok := payload.(*tasks.CreateInstance); ok {
					errs[tk.GetId()] = createInstance(c, tk)
				}
			}
			return errs
		}

		// runCountVMs runs the count-vms cron and executes all CountVMs tasks.
		runCountVMs := func() {
			resetScheduler()
			assert.Loosely(t, countVMsAsync(c), should.BeNil)
			countTasks := sched.Tasks().Payloads()
			resetScheduler()
			for _, payload := range countTasks {
				if _, ok := payload.(*tasks.CountVMs); ok {
					assert.Loosely(t, countVMs(c, payload), should.BeNil)
				}
			}
		}

		// makeFallbackConfig builds a VM pool config with {{.Zone}} templates in
		// MachineType and Disk.Type for testing both single-zone and multi-zone
		// fallback creation flows.
		makeFallbackConfig := func(prefix, primaryZone string, fallbackZones []string, amount int32) *config.Config {
			return &config.Config{
				Prefix:   prefix,
				Revision: "rev-1",
				Swarming: "chromium-swarm-dev.appspot.com",
				Owner:    []string{"owners"},
				Amount: &config.Amount{
					Min: amount,
					Max: amount,
				},
				Lifetime: &config.TimePeriod{
					Time: &config.TimePeriod_Seconds{Seconds: 3600},
				},
				Timeout: &config.TimePeriod{
					Time: &config.TimePeriod_Seconds{Seconds: 600},
				},
				Attributes: &config.VM{
					Project:       "test-project",
					Zone:          primaryZone,
					FallbackZones: fallbackZones,
					MachineType:   "zones/{{.Zone}}/machineTypes/n2-standard-8",
					Disk: []*config.Disk{
						{
							Image: "projects/test-project/global/images/chrome-bot",
							Type:  "zones/{{.Zone}}/diskTypes/pd-ssd",
							Size:  100,
						},
					},
				},
			}
		}

		// Scenario 1: Standard VM creation without fallback_zones.
		// Verifies that when a config has no fallback_zones (both a static
		// amount-based pool and a dynamic CloudBots DUT-based pool):
		//   - createVM expands {{.Zone}} placeholders immediately into the
		//     configured zone before saving model.VM in Datastore.
		//   - createInstance inserts each VM in the configured zone, fetches the
		//     created instance metadata (IPs, selfLink, creation timestamp), and
		//     persists URL and Created on model.VM.
		//   - Subsequent create-instances ticks skip already-created VMs, and
		//     countVMs records the created instance counts per zone.
		t.Run("simple VM creation flow without fallback zones (amount and DUT configs)", func(t *ftt.Test) {
			// 1. Receive a standard amount-based config (2 VMs) and a CloudBots DUT config (1 VM).
			receiveConfig(makeFallbackConfig("simple-pool", "us-central1-a", nil, 2))
			dutCfg := makeFallbackConfig("cloudbots-pool", "us-central1-a", nil, 0)
			dutCfg.Duts = map[string]*emptypb.Empty{"chromeos8-row1-rack2-host3": {}}
			receiveConfig(dutCfg)

			// 2. Expand configs into model.VM Datastore entities.
			runExpandAndCreateVMs()

			vm0 := &model.VM{ID: "simple-pool-0"}
			vm1 := &model.VM{ID: "simple-pool-1"}
			vmDUT := &model.VM{ID: "cloudbots-pool-c8-r1-r2-h3"}
			assert.Loosely(t, datastore.Get(c, vm0), should.BeNil)
			assert.Loosely(t, datastore.Get(c, vm1), should.BeNil)
			assert.Loosely(t, datastore.Get(c, vmDUT), should.BeNil)
			assert.Loosely(t, vm0.URL, should.BeEmpty)
			// Without fallback_zones, {{.Zone}} is expanded during createVM.
			assert.Loosely(t, vm0.Attributes.GetMachineType(), should.Equal("zones/us-central1-a/machineTypes/n2-standard-8"))
			assert.Loosely(t, vmDUT.DUT, should.Equal("chromeos8-row1-rack2-host3"))

			// 3. Run create-instances cron tick: GCE creates all 3 VMs in us-central1-a.
			errs := runCreateInstancesTick()
			assert.Loosely(t, len(errs), should.Equal(3))
			for _, err := range errs {
				assert.Loosely(t, err, should.BeNil)
			}

			assert.Loosely(t, datastore.Get(c, vm0), should.BeNil)
			assert.Loosely(t, datastore.Get(c, vm1), should.BeNil)
			assert.Loosely(t, datastore.Get(c, vmDUT), should.BeNil)
			assert.Loosely(t, vm0.URL, should.ContainSubstring("/zones/us-central1-a/instances/"+vm0.Hostname))
			assert.Loosely(t, vm1.URL, should.ContainSubstring("/zones/us-central1-a/instances/"+vm1.Hostname))
			assert.Loosely(t, vmDUT.URL, should.ContainSubstring("/zones/us-central1-a/instances/"+vmDUT.Hostname))
			assert.Loosely(t, vm0.Created, should.BeGreaterThan(int64(0)))
			assert.Loosely(t, vm0.FallbackZoneRotations, should.Equal(int64(0)))
			assert.Loosely(t, len(vm0.NetworkInterfaces), should.Equal(1))
			assert.Loosely(t, vm0.NetworkInterfaces[0].InternalIP, should.Equal("10.0.0.10"))

			// Next create-instances cron tick is a no-op since all VMs now have URLs.
			assert.Loosely(t, runCreateInstancesTick(), should.BeEmpty)

			// 4. Run count-vms cron tick: verifies 2 created VMs in simple-pool.
			runCountVMs()
			ic := &metrics.InstanceCount{ID: "simple-pool"}
			assert.Loosely(t, datastore.Get(c, ic), should.BeNil)
			assert.Loosely(t, len(ic.Created), should.Equal(1))
			assert.Loosely(t, ic.Created[0].Zone, should.Equal("us-central1-a"))
			assert.Loosely(t, ic.Created[0].Count, should.Equal(2))
		})

		// Scenario 2: Multi-zone fallback config succeeding on the root zone.
		// Verifies that when fallback_zones is configured and the root (primary)
		// zone has available capacity:
		//   - createVM preserves {{.Zone}} templates unexpanded in Datastore and
		//     records FallbackOriginalZone.
		//   - createInstance attempts the root zone first using requestId
		//     "create-<hostname>-fz-0", succeeds without rotating, and expands
		//     {{.Zone}} templates into the root zone inside setCreated.
		//   - countVMs reports 1 created instance in the root zone and 0 in each
		//     unused fallback zone so Monarch gauges zero-initialize all candidate
		//     zones.
		t.Run("fallback zones config succeeds on root zone when capacity is available", func(t *ftt.Test) {
			receiveConfig(makeFallbackConfig(
				"fb-root-ok",
				"us-central1-c",
				[]string{"us-central1-a", "us-central1-b"},
				1,
			))

			// Step 1: Expand config -> createVM keeps {{.Zone}} templates unexpanded.
			runExpandAndCreateVMs()
			vm := &model.VM{ID: "fb-root-ok-0"}
			assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
			assert.Loosely(t, vm.FallbackOriginalZone, should.Equal("us-central1-c"))
			assert.Loosely(t, vm.Attributes.GetZone(), should.Equal("us-central1-c"))
			assert.Loosely(t, vm.Attributes.GetMachineType(), should.Equal("zones/{{.Zone}}/machineTypes/n2-standard-8"))
			assert.Loosely(t, vm.Attributes.GetDisk()[0].GetType(), should.Equal("zones/{{.Zone}}/diskTypes/pd-ssd"))

			// Step 2: Create instance -> root zone us-central1-c succeeds on first try.
			errs := runCreateInstancesTick()
			assert.Loosely(t, errs["fb-root-ok-0"], should.BeNil)

			assert.Loosely(t, len(fakeGCE.calls), should.Equal(2))
			assert.Loosely(t, fakeGCE.calls[0].Method, should.Equal(http.MethodPost))
			assert.Loosely(t, fakeGCE.calls[0].Zone, should.Equal("us-central1-c"))
			assert.Loosely(t, fakeGCE.calls[0].MachineType, should.Equal("zones/us-central1-c/machineTypes/n2-standard-8"))
			assert.Loosely(t, fakeGCE.calls[0].RequestID, should.Equal(uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("create-%s-fz-0", vm.Hostname))).String()))
			assert.Loosely(t, fakeGCE.calls[1].Method, should.Equal(http.MethodGet))
			assert.Loosely(t, fakeGCE.calls[1].Zone, should.Equal("us-central1-c"))

			assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
			assert.Loosely(t, vm.Attributes.GetZone(), should.Equal("us-central1-c"))
			assert.Loosely(t, vm.FallbackOriginalZone, should.Equal("us-central1-c"))
			assert.Loosely(t, vm.FallbackFailedZones, should.BeEmpty)
			assert.Loosely(t, vm.FallbackZoneRotations, should.Equal(int64(0)))
			assert.Loosely(t, vm.Attributes.GetMachineType(), should.Equal("zones/us-central1-c/machineTypes/n2-standard-8"))
			assert.Loosely(t, vm.Attributes.GetDisk()[0].GetType(), should.Equal("zones/us-central1-c/diskTypes/pd-ssd"))

			// Step 3: Count VMs -> reports 1 in root zone and 0 in unused fallback zones.
			runCountVMs()
			ic := &metrics.InstanceCount{ID: "fb-root-ok"}
			assert.Loosely(t, datastore.Get(c, ic), should.BeNil)
			assert.Loosely(t, len(ic.Created), should.Equal(3))
			assert.Loosely(t, ic.Created[0].Zone, should.Equal("us-central1-c"))
			assert.Loosely(t, ic.Created[0].Count, should.Equal(1))
			assert.Loosely(t, ic.Created[1].Zone, should.Equal("us-central1-a"))
			assert.Loosely(t, ic.Created[1].Count, should.Equal(0))
			assert.Loosely(t, ic.Created[2].Zone, should.Equal("us-central1-b"))
			assert.Loosely(t, ic.Created[2].Count, should.Equal(0))
		})

		// Scenario 3: Multi-zone fallback rotation on root and intermediate zone stockouts.
		// Verifies that when the root zone (us-central1-c) fails with a synchronous
		// HTTP 503 stockout error and the first fallback zone (us-central1-a) fails
		// with an asynchronous DONE operation stockout
		// (ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS):
		//   - createInstance sequentially rotates across candidate zones within the
		//     same task execution (us-central1-c -> us-central1-a -> us-central1-b)
		//     using distinct rotation-scoped requestIds (fz-0, fz-1, fz-2).
		//   - Upon succeeding in us-central1-b, setCreated expands {{.Zone}} into
		//     us-central1-b, preserves FallbackOriginalZone ("us-central1-c"),
		//     records FallbackFailedZones (["us-central1-c", "us-central1-a"]), and
		//     sets FallbackZoneRotations to 2.
		//   - countVMs attributes the created instance to us-central1-b and reports
		//     0 for the stocked-out zones.
		t.Run("fallback zones config rotates when root zone fails with stockout and succeeds on fallback zone", func(t *ftt.Test) {
			// Root zone us-central1-c fails with HTTP 503 stockout message;
			// Fallback 1 (us-central1-a) fails with async operation ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS;
			// Fallback 2 (us-central1-b) succeeds.
			fakeGCE.scriptZone("us-central1-c", gceHTTPError(
				http.StatusServiceUnavailable,
				errCodeZoneResourcePoolExhausted,
				"The zone 'projects/test-project/zones/us-central1-c' does not have enough resources available to fulfill the request.",
			))
			fakeGCE.scriptZone("us-central1-a", gceOpError(
				errCodeZoneResourcePoolExhaustedWithDetails,
				"The zone 'projects/test-project/zones/us-central1-a' does not have enough resources available to fulfill the request.",
			))
			fakeGCE.scriptZone("us-central1-b", gceDone())

			receiveConfig(makeFallbackConfig(
				"fb-rotate",
				"us-central1-c",
				[]string{"us-central1-a", "us-central1-b"},
				1,
			))
			runExpandAndCreateVMs()

			vm := &model.VM{ID: "fb-rotate-0"}
			assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
			host := vm.Hostname

			errs := runCreateInstancesTick()
			assert.Loosely(t, errs["fb-rotate-0"], should.BeNil)

			// Verify GCE received 3 Insert calls (c -> a -> b) with incrementing fz-N requestIds,
			// followed by 1 Get call in the winning fallback zone us-central1-b.
			assert.Loosely(t, len(fakeGCE.calls), should.Equal(4))
			assert.Loosely(t, fakeGCE.calls[0].Zone, should.Equal("us-central1-c"))
			assert.Loosely(t, fakeGCE.calls[0].MachineType, should.Equal("zones/us-central1-c/machineTypes/n2-standard-8"))
			assert.Loosely(t, fakeGCE.calls[0].RequestID, should.Equal(uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("create-%s-fz-0", host))).String()))

			assert.Loosely(t, fakeGCE.calls[1].Zone, should.Equal("us-central1-a"))
			assert.Loosely(t, fakeGCE.calls[1].MachineType, should.Equal("zones/us-central1-a/machineTypes/n2-standard-8"))
			assert.Loosely(t, fakeGCE.calls[1].RequestID, should.Equal(uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("create-%s-fz-1", host))).String()))

			assert.Loosely(t, fakeGCE.calls[2].Zone, should.Equal("us-central1-b"))
			assert.Loosely(t, fakeGCE.calls[2].MachineType, should.Equal("zones/us-central1-b/machineTypes/n2-standard-8"))
			assert.Loosely(t, fakeGCE.calls[2].RequestID, should.Equal(uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("create-%s-fz-2", host))).String()))

			assert.Loosely(t, fakeGCE.calls[3].Method, should.Equal(http.MethodGet))
			assert.Loosely(t, fakeGCE.calls[3].Zone, should.Equal("us-central1-b"))

			assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
			assert.Loosely(t, vm.Attributes.GetZone(), should.Equal("us-central1-b"))
			assert.Loosely(t, vm.FallbackOriginalZone, should.Equal("us-central1-c"))
			assert.Loosely(t, vm.FallbackFailedZones, should.Match([]string{"us-central1-c", "us-central1-a"}))
			assert.Loosely(t, vm.FallbackZoneRotations, should.Equal(int64(2)))
			assert.Loosely(t, vm.Attributes.GetMachineType(), should.Equal("zones/us-central1-b/machineTypes/n2-standard-8"))
			assert.Loosely(t, vm.Attributes.GetDisk()[0].GetType(), should.Equal("zones/us-central1-b/diskTypes/pd-ssd"))
			assert.Loosely(t, vm.URL, should.ContainSubstring("/zones/us-central1-b/instances/"+host))

			runCountVMs()
			ic := &metrics.InstanceCount{ID: "fb-rotate"}
			assert.Loosely(t, datastore.Get(c, ic), should.BeNil)
			assert.Loosely(t, ic.Created[0].Zone, should.Equal("us-central1-c"))
			assert.Loosely(t, ic.Created[0].Count, should.Equal(0))
			assert.Loosely(t, ic.Created[1].Zone, should.Equal("us-central1-a"))
			assert.Loosely(t, ic.Created[1].Count, should.Equal(0))
			assert.Loosely(t, ic.Created[2].Zone, should.Equal("us-central1-b"))
			assert.Loosely(t, ic.Created[2].Count, should.Equal(1))
		})

		// Scenario 4: Transient HTTP 503 backendError during in-flight RUNNING operation.
		// Reproduces b/574158175: when an InsertInstance call in a fallback zone
		// (us-central1-a) creates the VM and returns RUNNING on Tick 1, and the
		// follow-up poll on Tick 2 encounters a transient HTTP 503 backendError:
		//   - handleInsertError must classify the 503 backendError as a transient
		//     retryable error rather than a capacity stockout, keeping the active
		//     zone cursor on us-central1-a and leaving FallbackZoneRotations at 1.
		//   - On Tick 3, createInstance retries us-central1-a with the exact same
		//     requestId ("create-<hostname>-fz-1"), allowing fakeGCEServer to
		//     return the completed DONE operation without triggering an HTTP 409
		//     alreadyExists conflict or leaking the running VM.
		t.Run("special GCE responses: transient HTTP 503 backendError after RUNNING retries same zone without 409 conflict", func(t *ftt.Test) {
			// Root zone us-central1-c is stocked out;
			// Fallback zone us-central1-a starts creating the VM (RUNNING) on Tick 1,
			// returns a transient HTTP 503 backendError on Tick 2, and returns DONE on Tick 3.
			fakeGCE.scriptZone("us-central1-c", gceOpError(
				errCodeZoneResourcePoolExhausted,
				"The zone 'projects/test-project/zones/us-central1-c' does not have enough resources available to fulfill the request.",
			))
			fakeGCE.scriptZone("us-central1-a",
				gceRunning(),
				gceHTTPError(http.StatusServiceUnavailable, "backendError", "Internal error. Please try again or contact Google Support."),
				gceDone(),
			)

			receiveConfig(makeFallbackConfig(
				"fb-503-backend",
				"us-central1-c",
				[]string{"us-central1-a", "us-central1-b"},
				1,
			))
			runExpandAndCreateVMs()

			vm := &model.VM{ID: "fb-503-backend-0"}
			assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
			host := vm.Hostname
			expectedReqID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("create-%s-fz-1", host))).String()

			// Tick 1: us-central1-c stocks out -> rotates to us-central1-a (RUNNING).
			errs := runCreateInstancesTick()
			assert.Loosely(t, errs["fb-503-backend-0"], should.BeNil)
			assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
			assert.Loosely(t, vm.Attributes.GetZone(), should.Equal("us-central1-a"))
			assert.Loosely(t, vm.FallbackZoneRotations, should.Equal(int64(1)))
			assert.Loosely(t, vm.URL, should.BeEmpty)

			// Tick 2: polling us-central1-a hits HTTP 503 backendError.
			// Must NOT rotate to us-central1-b or increment FallbackZoneRotations.
			errs = runCreateInstancesTick()
			assert.Loosely(t, errs["fb-503-backend-0"], should.ErrLike("transiently failed to create instance"))
			assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
			assert.Loosely(t, vm.Attributes.GetZone(), should.Equal("us-central1-a"))
			assert.Loosely(t, vm.FallbackZoneRotations, should.Equal(int64(1)))
			assert.Loosely(t, vm.Attributes.GetMachineType(), should.Equal("zones/{{.Zone}}/machineTypes/n2-standard-8"))

			// Tick 3: retries us-central1-a with the exact same requestId (fz-1) -> DONE (no 409 alreadyExists!).
			errs = runCreateInstancesTick()
			assert.Loosely(t, errs["fb-503-backend-0"], should.BeNil)
			assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
			assert.Loosely(t, vm.Attributes.GetZone(), should.Equal("us-central1-a"))
			assert.Loosely(t, vm.FallbackZoneRotations, should.Equal(int64(1)))
			assert.Loosely(t, vm.URL, should.ContainSubstring("/zones/us-central1-a/instances/"+host))

			// Verify all 3 Insert calls in us-central1-a used the exact same requestId.
			assert.Loosely(t, len(fakeGCE.calls), should.Equal(5))
			assert.Loosely(t, fakeGCE.calls[1].Zone, should.Equal("us-central1-a"))
			assert.Loosely(t, fakeGCE.calls[1].RequestID, should.Equal(expectedReqID))
			assert.Loosely(t, fakeGCE.calls[2].Zone, should.Equal("us-central1-a"))
			assert.Loosely(t, fakeGCE.calls[2].RequestID, should.Equal(expectedReqID))
			assert.Loosely(t, fakeGCE.calls[3].Zone, should.Equal("us-central1-a"))
			assert.Loosely(t, fakeGCE.calls[3].RequestID, should.Equal(expectedReqID))
		})

		// Scenario 5: Multi-tick asynchronous RUNNING operations with full wrap-around.
		// Verifies that when an asynchronous RUNNING operation in the root zone
		// later fails with ZONE_RESOURCE_POOL_EXHAUSTED on Tick 2, and all
		// remaining fallback zones also fail with stockout:
		//   - createInstance wraps the zone cursor back around to
		//     FallbackOriginalZone ("us-central1-c"), clears FallbackFailedZones
		//     for the new pass, preserves {{.Zone}} templates, and increments
		//     FallbackZoneRotations to 2.
		//   - On Tick 3, createInstance retries the root zone with a fresh
		//     rotation-scoped requestId ("create-<hostname>-fz-2") so GCE does not
		//     replay the cached failed operation from Tick 2, succeeding once
		//     capacity becomes available.
		t.Run("special GCE responses: multi-tick async RUNNING operations and wrap-around back to root zone", func(t *ftt.Test) {
			// Pass 1 across 2 ticks:
			// - us-central1-c returns RUNNING on Tick 1, then ZONE_RESOURCE_POOL_EXHAUSTED on Tick 2.
			// - us-central1-a returns ZONE_RESOURCE_POOL_EXHAUSTED on Tick 2 -> all zones exhausted -> wraps back to us-central1-c.
			// Pass 2 on Tick 3:
			// - us-central1-c now has capacity and returns DONE.
			fakeGCE.scriptZone("us-central1-c",
				gceRunning(),
				gceOpError(errCodeZoneResourcePoolExhausted, "The zone does not have enough resources available."),
				gceDone(),
			)
			fakeGCE.scriptZone("us-central1-a",
				gceOpError(errCodeZoneResourcePoolExhausted, "The zone does not have enough resources available."),
			)

			receiveConfig(makeFallbackConfig(
				"fb-wrap",
				"us-central1-c",
				[]string{"us-central1-a"},
				1,
			))
			runExpandAndCreateVMs()

			// Tick 1: root zone us-central1-c is RUNNING.
			errs := runCreateInstancesTick()
			assert.Loosely(t, errs["fb-wrap-0"], should.BeNil)

			// Tick 2: root zone fails with async stockout -> rotates to us-central1-a -> also fails -> wraps back to us-central1-c.
			errs = runCreateInstancesTick()
			assert.Loosely(t, errs["fb-wrap-0"], should.ErrLike("all candidate zones exhausted, rotating back to original zone us-central1-c"))

			vm := &model.VM{ID: "fb-wrap-0"}
			assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
			assert.Loosely(t, vm.Attributes.GetZone(), should.Equal("us-central1-c"))
			assert.Loosely(t, vm.FallbackOriginalZone, should.Equal("us-central1-c"))
			assert.Loosely(t, vm.FallbackFailedZones, should.BeEmpty)
			assert.Loosely(t, vm.FallbackZoneRotations, should.Equal(int64(2)))
			assert.Loosely(t, vm.Attributes.GetMachineType(), should.Equal("zones/{{.Zone}}/machineTypes/n2-standard-8"))

			// Tick 3: retries root zone us-central1-c with new requestId (fz-2) and succeeds.
			errs = runCreateInstancesTick()
			assert.Loosely(t, errs["fb-wrap-0"], should.BeNil)

			assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
			assert.Loosely(t, vm.Attributes.GetZone(), should.Equal("us-central1-c"))
			assert.Loosely(t, vm.FallbackZoneRotations, should.Equal(int64(2)))
			assert.Loosely(t, vm.Attributes.GetMachineType(), should.Equal("zones/us-central1-c/machineTypes/n2-standard-8"))
			assert.Loosely(t, vm.URL, should.ContainSubstring("/zones/us-central1-c/instances/"+vm.Hostname))
		})

		// Scenario 6: Permanent non-stockout GCE operation error (QUOTA_EXCEEDED).
		// Verifies that when GCE returns a non-stockout operation error (such as
		// regional QUOTA_EXCEEDED):
		//   - handleOperationErrors does not rotate to sibling zones in the same
		//     region and instead deletes the unassigned model.VM entity from
		//     Datastore.
		//   - The next expand-configs and create-instances cycle recreates a fresh
		//     model.VM entity that succeeds in the root zone once quota is
		//     available.
		t.Run("special GCE responses: permanent non-stockout error deletes VM without fallback and recreates on next expand", func(t *ftt.Test) {
			// Attempt 1 in root zone us-central1-c fails with QUOTA_EXCEEDED (non-stockout),
			// Attempt 2 succeeds once quota is available.
			fakeGCE.scriptZone("us-central1-c",
				gceOpError(errCodeQuotaExceeded, "Quota 'CPUS' exceeded. Limit: 100.0 in region us-central1."),
				gceDone(),
			)

			receiveConfig(makeFallbackConfig(
				"fb-quota",
				"us-central1-c",
				[]string{"us-central1-a"},
				1,
			))
			runExpandAndCreateVMs()

			// Tick 1: QUOTA_EXCEEDED does not rotate to us-central1-a; deletes the failed VM entity.
			errs := runCreateInstancesTick()
			assert.Loosely(t, errs["fb-quota-0"], should.ErrLike("failed to create instance"))
			assert.Loosely(t, len(fakeGCE.calls), should.Equal(1))
			assert.Loosely(t, fakeGCE.calls[0].Zone, should.Equal("us-central1-c"))
			assert.Loosely(t, datastore.Get(c, &model.VM{ID: "fb-quota-0"}), should.Equal(datastore.ErrNoSuchEntity))

			// Next expand-configs + create-instances cycle creates a fresh VM entity that succeeds in us-central1-c.
			runExpandAndCreateVMs()
			errs = runCreateInstancesTick()
			assert.Loosely(t, errs["fb-quota-0"], should.BeNil)

			vm := &model.VM{ID: "fb-quota-0"}
			assert.Loosely(t, datastore.Get(c, vm), should.BeNil)
			assert.Loosely(t, vm.Attributes.GetZone(), should.Equal("us-central1-c"))
			assert.Loosely(t, vm.FallbackZoneRotations, should.Equal(int64(0)))
			assert.Loosely(t, vm.URL, should.ContainSubstring("/zones/us-central1-c/instances/"+vm.Hostname))
		})
	})
}
