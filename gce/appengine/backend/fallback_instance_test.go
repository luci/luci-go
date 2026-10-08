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
	"net/http"
	"reflect"
	"testing"

	"google.golang.org/api/compute/v1"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"
)

func TestFallbackInstance(t *testing.T) {
	t.Parallel()

	ftt.Run("fallback_instance", t, func(t *ftt.Test) {
		dsp := &tq.Dispatcher{}
		registerTasks(dsp)
		rt := &roundtripper.JSONRoundTripper{}
		gce, err := compute.New(&http.Client{Transport: rt})
		assert.Loosely(t, err, should.BeNil)
		c, _ := tq.TestingContext(memory.Use(context.Background()), dsp)
		c = withCompute(withDispatcher(c, dsp), ComputeService{Stable: gce})
		c, _ = tsmon.WithDummyInMemory(c)

		t.Run("zone fallback helpers", func(t *ftt.Test) {
			vm := &model.VM{
				ID:                   "id",
				Hostname:             "name",
				FallbackOriginalZone: "us-central1-c",
				Attributes: config.VM{
					Zone:          "us-central1-c",
					FallbackZones: []string{"us-central1-a", "us-central1-b", "us-central1-f"},
					MachineType:   "zones/{{.Zone}}/machineTypes/n2-standard-8",
					Disk: []*config.Disk{{
						Image: "global/images/image",
						Type:  "zones/{{.Zone}}/diskTypes/pd-ssd",
					}},
				},
			}
			assert.Loosely(t, candidateZones(vm), should.Match([]string{"us-central1-c", "us-central1-a", "us-central1-b", "us-central1-f"}))

			// FallbackFailedZones skips previously exhausted zones for this VM instance.
			vm.FallbackFailedZones = []string{"us-central1-c", "us-central1-a"}
			vm.Attributes.Zone = "us-central1-b"
			assert.Loosely(t, candidateZones(vm), should.Match([]string{"us-central1-b", "us-central1-f"}))

			// When all candidate zones have failed, candidateZones wraps around to the
			// full list starting from FallbackOriginalZone.
			vm.FallbackFailedZones = []string{"us-central1-c", "us-central1-a", "us-central1-b", "us-central1-f"}
			vm.Attributes.Zone = "us-central1-c"
			assert.Loosely(t, candidateZones(vm), should.Match([]string{"us-central1-c", "us-central1-a", "us-central1-b", "us-central1-f"}))
			vm.FallbackFailedZones = nil

			cloned := cloneVMForZone(vm, "us-central1-a")
			assert.Loosely(t, cloned.Attributes.GetZone(), should.Equal("us-central1-a"))
			assert.Loosely(t, cloned.Attributes.GetMachineType(), should.Equal("zones/us-central1-a/machineTypes/n2-standard-8"))
			assert.Loosely(t, vm.Attributes.GetMachineType(), should.Equal("zones/{{.Zone}}/machineTypes/n2-standard-8"))

			rID0 := createInstanceRequestID(vm, "us-central1-c")
			vm.FallbackZoneRotations = 3
			rID3 := createInstanceRequestID(vm, "us-central1-c")
			assert.Loosely(t, rID0, should.NotEqual(rID3))
			vm.FallbackZoneRotations = 0

			assert.Loosely(t, datastore.Put(c, vm), should.BeNil)
			assert.Loosely(t, setActiveVMZone(c, "id", "us-central1-a", "us-central1-c", []string{"us-central1-c"}, 1), should.BeNil)
			stored := &model.VM{ID: "id"}
			assert.Loosely(t, datastore.Get(c, stored), should.BeNil)
			assert.Loosely(t, stored.FallbackOriginalZone, should.Equal("us-central1-c"))
			assert.Loosely(t, stored.FallbackFailedZones, should.Match([]string{"us-central1-c"}))
			assert.Loosely(t, stored.FallbackZoneRotations, should.Equal(int64(1)))
			assert.Loosely(t, stored.Attributes.GetZone(), should.Equal("us-central1-a"))
			assert.Loosely(t, stored.Attributes.GetMachineType(), should.Equal("zones/{{.Zone}}/machineTypes/n2-standard-8"))

			assert.Loosely(t, updateVMZone(c, "id", "us-central1-b", "us-central1-c", []string{"us-central1-c", "us-central1-a"}, 2), should.BeNil)
			stored = &model.VM{ID: "id"}
			assert.Loosely(t, datastore.Get(c, stored), should.BeNil)
			assert.Loosely(t, stored.FallbackOriginalZone, should.Equal("us-central1-c"))
			assert.Loosely(t, stored.FallbackFailedZones, should.Match([]string{"us-central1-c", "us-central1-a"}))
			assert.Loosely(t, stored.FallbackZoneRotations, should.Equal(int64(2)))
			assert.Loosely(t, stored.Attributes.GetZone(), should.Equal("us-central1-b"))
			assert.Loosely(t, stored.Attributes.GetMachineType(), should.Equal("zones/us-central1-b/machineTypes/n2-standard-8"))
			assert.Loosely(t, stored.AttributesIndexed, should.Match([]string{"disk.image:image"}))
		})

		t.Run("createInstance with fallback zones", func(t *ftt.Test) {
			makeVM := func() *model.VM {
				return &model.VM{
					ID:       "id",
					Hostname: "name",
					Prefix:   "prefix",
					Attributes: config.VM{
						Project:       "project",
						Zone:          "us-central1-c",
						FallbackZones: []string{"us-central1-a", "us-central1-b"},
						Disk:          []*config.Disk{{Type: "zones/{{.Zone}}/diskTypes/pd-ssd"}},
						MachineType:   "zones/{{.Zone}}/machineTypes/n2-standard-8",
					},
				}
			}
			opErr := func(code, msg string) *compute.Operation {
				return &compute.Operation{
					Error: &compute.OperationError{
						Errors: []*compute.OperationErrorErrors{{Code: code, Message: msg}},
					},
				}
			}
			opDone := func(link string) *compute.Operation {
				return &compute.Operation{
					EndTime:    "2018-12-14T15:07:48.200-08:00",
					Status:     "DONE",
					TargetLink: link,
				}
			}

			t.Run("http 503 stockout falls back and succeeds", func(t *ftt.Test) {
				attempts := 0
				rt.Handler = func(req any) (int, any) {
					attempts++
					switch attempts {
					case 1:
						inst := req.(*compute.Instance)
						assert.Loosely(t, inst.MachineType, should.Equal("zones/us-central1-c/machineTypes/n2-standard-8"))
						assert.Loosely(t, inst.Disks[0].InitializeParams.DiskType, should.Equal("zones/us-central1-c/diskTypes/pd-ssd"))
						return http.StatusServiceUnavailable, nil
					case 2:
						inst := req.(*compute.Instance)
						assert.Loosely(t, inst.MachineType, should.Equal("zones/us-central1-a/machineTypes/n2-standard-8"))
						assert.Loosely(t, inst.Disks[0].InitializeParams.DiskType, should.Equal("zones/us-central1-a/diskTypes/pd-ssd"))
						rt.Type = reflect.TypeOf(map[string]string{})
						return http.StatusOK, opDone("url-fallback-a")
					default:
						return http.StatusOK, &compute.Instance{
							CreationTimestamp: "2018-12-14T15:07:48.200-08:00",
							SelfLink:          "url-fallback-a",
						}
					}
				}
				rt.Type = reflect.TypeOf(compute.Instance{})
				assert.Loosely(t, datastore.Put(c, makeVM()), should.BeNil)
				assert.Loosely(t, createInstance(c, &tasks.CreateInstance{Id: "id"}), should.BeNil)
				assert.Loosely(t, attempts, should.Equal(3))

				v := &model.VM{ID: "id"}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.FallbackOriginalZone, should.Equal("us-central1-c"))
				assert.Loosely(t, v.FallbackFailedZones, should.Match([]string{"us-central1-c"}))
				assert.Loosely(t, v.FallbackZoneRotations, should.Equal(int64(1)))
				assert.Loosely(t, v.Attributes.GetZone(), should.Equal("us-central1-a"))
				assert.Loosely(t, v.Attributes.GetMachineType(), should.Equal("zones/us-central1-a/machineTypes/n2-standard-8"))
				assert.Loosely(t, v.Attributes.GetDisk()[0].GetType(), should.Equal("zones/us-central1-a/diskTypes/pd-ssd"))
				assert.Loosely(t, v.URL, should.Equal("url-fallback-a"))
			})

			t.Run("operation stockout falls back across multiple zones", func(t *ftt.Test) {
				attempts := 0
				rt.Handler = func(req any) (int, any) {
					attempts++
					switch attempts {
					case 1:
						return http.StatusOK, opErr(errCodeZoneResourcePoolExhausted, "The zone "+errMsgZoneResourcePoolExhausted+".")
					case 2:
						return http.StatusOK, opErr(errCodeZoneResourcePoolExhaustedWithDetails, "The zone "+errMsgZoneResourcePoolExhausted+".")
					case 3:
						inst := req.(*compute.Instance)
						assert.Loosely(t, inst.MachineType, should.Equal("zones/us-central1-b/machineTypes/n2-standard-8"))
						rt.Type = reflect.TypeOf(map[string]string{})
						return http.StatusOK, opDone("url-fallback-b")
					default:
						return http.StatusOK, &compute.Instance{
							CreationTimestamp: "2018-12-14T15:07:48.200-08:00",
							SelfLink:          "url-fallback-b",
						}
					}
				}
				rt.Type = reflect.TypeOf(compute.Instance{})
				assert.Loosely(t, datastore.Put(c, makeVM()), should.BeNil)
				assert.Loosely(t, createInstance(c, &tasks.CreateInstance{Id: "id"}), should.BeNil)
				assert.Loosely(t, attempts, should.Equal(4))

				v := &model.VM{ID: "id"}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.FallbackOriginalZone, should.Equal("us-central1-c"))
				assert.Loosely(t, v.FallbackFailedZones, should.Match([]string{"us-central1-c", "us-central1-a"}))
				assert.Loosely(t, v.FallbackZoneRotations, should.Equal(int64(2)))
				assert.Loosely(t, v.Attributes.GetZone(), should.Equal("us-central1-b"))
				assert.Loosely(t, v.URL, should.Equal("url-fallback-b"))
			})

			t.Run("all candidate zones exhausted rotates back to original zone and retries", func(t *ftt.Test) {
				attempts := 0
				rt.Handler = func(req any) (int, any) {
					attempts++
					switch attempts {
					case 1, 2, 3:
						return http.StatusServiceUnavailable, nil
					case 4:
						// Second pass starts back at original zone us-central1-c, which is now free.
						inst := req.(*compute.Instance)
						assert.Loosely(t, inst.MachineType, should.Equal("zones/us-central1-c/machineTypes/n2-standard-8"))
						assert.Loosely(t, inst.Disks[0].InitializeParams.DiskType, should.Equal("zones/us-central1-c/diskTypes/pd-ssd"))
						rt.Type = reflect.TypeOf(map[string]string{})
						return http.StatusOK, opDone("url-primary-c")
					default:
						return http.StatusOK, &compute.Instance{
							CreationTimestamp: "2018-12-14T15:07:48.200-08:00",
							SelfLink:          "url-primary-c",
						}
					}
				}
				rt.Type = reflect.TypeOf(compute.Instance{})
				assert.Loosely(t, datastore.Put(c, makeVM()), should.BeNil)

				// Pass 1: all 3 zones fail -> VM wraps back to original zone us-central1-c.
				assert.Loosely(t, createInstance(c, &tasks.CreateInstance{Id: "id"}), should.ErrLike("rotating back to original zone us-central1-c"))
				assert.Loosely(t, attempts, should.Equal(3))

				v := &model.VM{ID: "id"}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.FallbackOriginalZone, should.Equal("us-central1-c"))
				assert.Loosely(t, v.Attributes.GetZone(), should.Equal("us-central1-c"))
				assert.Loosely(t, v.FallbackFailedZones, should.BeEmpty)
				assert.Loosely(t, v.FallbackZoneRotations, should.Equal(int64(3)))
				assert.Loosely(t, v.Attributes.GetMachineType(), should.Equal("zones/{{.Zone}}/machineTypes/n2-standard-8"))

				// Pass 2: retries original zone us-central1-c and succeeds.
				assert.Loosely(t, createInstance(c, &tasks.CreateInstance{Id: "id"}), should.BeNil)
				assert.Loosely(t, attempts, should.Equal(5))

				v = &model.VM{ID: "id"}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.FallbackOriginalZone, should.Equal("us-central1-c"))
				assert.Loosely(t, v.Attributes.GetZone(), should.Equal("us-central1-c"))
				assert.Loosely(t, v.FallbackZoneRotations, should.Equal(int64(3)))
				assert.Loosely(t, v.Attributes.GetMachineType(), should.Equal("zones/us-central1-c/machineTypes/n2-standard-8"))
				assert.Loosely(t, v.URL, should.Equal("url-primary-c"))
			})

			t.Run("non-stockout operation error does not fallback", func(t *ftt.Test) {
				attempts := 0
				rt.Handler = func(req any) (int, any) {
					attempts++
					return http.StatusOK, opErr(errCodeQuotaExceeded, "Quota 'CPUS' exceeded.")
				}
				rt.Type = reflect.TypeOf(compute.Instance{})
				assert.Loosely(t, datastore.Put(c, makeVM()), should.BeNil)
				assert.Loosely(t, createInstance(c, &tasks.CreateInstance{Id: "id"}), should.ErrLike("failed to create instance"))
				assert.Loosely(t, attempts, should.Equal(1))
				assert.Loosely(t, datastore.Get(c, &model.VM{ID: "id"}), should.Equal(datastore.ErrNoSuchEntity))
			})

			t.Run("multi-tick async operation stockout falls back and preserves templates", func(t *ftt.Test) {
				attempts := 0
				rt.Handler = func(req any) (int, any) {
					attempts++
					switch attempts {
					case 1:
						// Tick 1: primary zone us-central1-c starts async operation (RUNNING).
						inst := req.(*compute.Instance)
						assert.Loosely(t, inst.MachineType, should.Equal("zones/us-central1-c/machineTypes/n2-standard-8"))
						assert.Loosely(t, inst.Disks[0].InitializeParams.DiskType, should.Equal("zones/us-central1-c/diskTypes/pd-ssd"))
						return http.StatusOK, &compute.Operation{Status: "RUNNING"}
					case 2:
						// Tick 2: polling us-central1-c returns async stockout error.
						inst := req.(*compute.Instance)
						assert.Loosely(t, inst.MachineType, should.Equal("zones/us-central1-c/machineTypes/n2-standard-8"))
						return http.StatusOK, opErr(errCodeZoneResourcePoolExhausted, "The zone "+errMsgZoneResourcePoolExhausted+".")
					case 3:
						// Tick 2 (continued): rotates to fallback 1 (us-central1-a), starts async operation (RUNNING).
						inst := req.(*compute.Instance)
						assert.Loosely(t, inst.MachineType, should.Equal("zones/us-central1-a/machineTypes/n2-standard-8"))
						assert.Loosely(t, inst.Disks[0].InitializeParams.DiskType, should.Equal("zones/us-central1-a/diskTypes/pd-ssd"))
						return http.StatusOK, &compute.Operation{Status: "RUNNING"}
					case 4:
						// Tick 3: resumes directly at us-central1-a (does not retry us-central1-c), returns async stockout.
						inst := req.(*compute.Instance)
						assert.Loosely(t, inst.MachineType, should.Equal("zones/us-central1-a/machineTypes/n2-standard-8"))
						return http.StatusOK, opErr(errCodeZoneResourcePoolExhaustedWithDetails, "The zone "+errMsgZoneResourcePoolExhausted+".")
					case 5:
						// Tick 3 (continued): rotates to fallback 2 (us-central1-b), starts async operation (RUNNING).
						inst := req.(*compute.Instance)
						assert.Loosely(t, inst.MachineType, should.Equal("zones/us-central1-b/machineTypes/n2-standard-8"))
						assert.Loosely(t, inst.Disks[0].InitializeParams.DiskType, should.Equal("zones/us-central1-b/diskTypes/pd-ssd"))
						return http.StatusOK, &compute.Operation{Status: "RUNNING"}
					case 6:
						// Tick 4: resumes directly at us-central1-b, operation completes (DONE).
						inst := req.(*compute.Instance)
						assert.Loosely(t, inst.MachineType, should.Equal("zones/us-central1-b/machineTypes/n2-standard-8"))
						rt.Type = reflect.TypeOf(map[string]string{})
						return http.StatusOK, opDone("url-fallback-b")
					default:
						return http.StatusOK, &compute.Instance{
							CreationTimestamp: "2018-12-14T15:07:48.200-08:00",
							SelfLink:          "url-fallback-b",
						}
					}
				}
				rt.Type = reflect.TypeOf(compute.Instance{})
				assert.Loosely(t, datastore.Put(c, makeVM()), should.BeNil)

				// Tick 1: primary zone RUNNING.
				assert.Loosely(t, createInstance(c, &tasks.CreateInstance{Id: "id"}), should.BeNil)
				assert.Loosely(t, attempts, should.Equal(1))
				v := &model.VM{ID: "id"}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.Attributes.GetZone(), should.Equal("us-central1-c"))
				assert.Loosely(t, v.Attributes.GetMachineType(), should.Equal("zones/{{.Zone}}/machineTypes/n2-standard-8"))

				// Tick 2: primary zone fails with stockout -> fallback 1 (us-central1-a) RUNNING.
				// Active zone cursor advances, while {{.Zone}} templates remain unexpanded in Datastore.
				assert.Loosely(t, createInstance(c, &tasks.CreateInstance{Id: "id"}), should.BeNil)
				assert.Loosely(t, attempts, should.Equal(3))
				v = &model.VM{ID: "id"}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.FallbackOriginalZone, should.Equal("us-central1-c"))
				assert.Loosely(t, v.FallbackFailedZones, should.Match([]string{"us-central1-c"}))
				assert.Loosely(t, v.FallbackZoneRotations, should.Equal(int64(1)))
				assert.Loosely(t, v.Attributes.GetZone(), should.Equal("us-central1-a"))
				assert.Loosely(t, v.Attributes.GetMachineType(), should.Equal("zones/{{.Zone}}/machineTypes/n2-standard-8"))
				assert.Loosely(t, v.Attributes.GetDisk()[0].GetType(), should.Equal("zones/{{.Zone}}/diskTypes/pd-ssd"))

				// Tick 3: fallback 1 fails with stockout -> fallback 2 (us-central1-b) RUNNING.
				assert.Loosely(t, createInstance(c, &tasks.CreateInstance{Id: "id"}), should.BeNil)
				assert.Loosely(t, attempts, should.Equal(5))
				v = &model.VM{ID: "id"}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.FallbackOriginalZone, should.Equal("us-central1-c"))
				assert.Loosely(t, v.FallbackFailedZones, should.Match([]string{"us-central1-c", "us-central1-a"}))
				assert.Loosely(t, v.FallbackZoneRotations, should.Equal(int64(2)))
				assert.Loosely(t, v.Attributes.GetZone(), should.Equal("us-central1-b"))
				assert.Loosely(t, v.Attributes.GetMachineType(), should.Equal("zones/{{.Zone}}/machineTypes/n2-standard-8"))

				// Tick 4: fallback 2 completes (DONE) -> templates are finalized via SetZone.
				assert.Loosely(t, createInstance(c, &tasks.CreateInstance{Id: "id"}), should.BeNil)
				assert.Loosely(t, attempts, should.Equal(7))
				v = &model.VM{ID: "id"}
				assert.Loosely(t, datastore.Get(c, v), should.BeNil)
				assert.Loosely(t, v.FallbackOriginalZone, should.Equal("us-central1-c"))
				assert.Loosely(t, v.FallbackFailedZones, should.Match([]string{"us-central1-c", "us-central1-a"}))
				assert.Loosely(t, v.FallbackZoneRotations, should.Equal(int64(2)))
				assert.Loosely(t, v.Attributes.GetZone(), should.Equal("us-central1-b"))
				assert.Loosely(t, v.Attributes.GetMachineType(), should.Equal("zones/us-central1-b/machineTypes/n2-standard-8"))
				assert.Loosely(t, v.Attributes.GetDisk()[0].GetType(), should.Equal("zones/us-central1-b/diskTypes/pd-ssd"))
				assert.Loosely(t, v.URL, should.Equal("url-fallback-b"))
			})
		})
	})
}
