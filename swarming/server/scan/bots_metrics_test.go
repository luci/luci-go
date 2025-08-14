// Copyright 2024 The LUCI Authors.
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

package scan

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/gae/impl/memory"
)

func TestBotsMetricsReporter(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())

	mon := &monitor.Fake{
		CS: 10000, // send all values via one chunk
	}

	r := BotsMetricsReporter{
		ServiceName: "service",
		JobName:     "job",
		Monitor:     mon,
	}

	err := RunBotVisitor(ctx, &r, []FakeBot{
		{
			ID:          "healthy-0",
			Pool:        []string{"pool-0", "pool-1"},
			OS:          []string{"Ubuntu", "Ubuntu-22"},
			RBEInstance: "rbe-0",
			Version:     "v0",
		},
		{
			ID:          "healthy-1",
			Pool:        []string{"pool-0", "pool-1"},
			OS:          []string{"Ubuntu", "Ubuntu-22"},
			RBEInstance: "rbe-0",
			Busy:        true,
			Version:     "v0",
		},
		{
			ID:            "healthy-2",
			Pool:          []string{"pool-0", "pool-1"},
			OS:            []string{"Ubuntu", "Ubuntu-22"},
			RBEInstance:   "rbe-0",
			RBEHybridMode: true,
			Version:       "v0",
		},
		{
			ID:          "healthy-3",
			Pool:        []string{"pool-0", "pool-1"},
			OS:          []string{"Ubuntu", "Ubuntu-22"},
			RBEInstance: "rbe-0",
			Handshaking: true,
			Version:     "v1",
		},
		{
			ID:          "healthy-4",
			Pool:        []string{"pool-0", "pool-1"},
			OS:          []string{"Ubuntu", "Ubuntu-22"},
			RBEInstance: "rbe-1",
			Version:     "v1",
		},
		{
			ID:      "healthy-5",
			Pool:    []string{"pool-0", "pool-1"},
			OS:      []string{"Ubuntu", "Ubuntu-22"},
			Version: "v1",
		},
		{
			ID:          "quarantined",
			Pool:        []string{"pool-0"},
			Quarantined: true,
			RBEInstance: "rbe-0",
			Version:     "v2",
		},
		{
			ID:          "maintenance",
			Pool:        []string{"pool-0"},
			Maintenance: true,
			RBEInstance: "rbe-0",
			Version:     "v2",
		},
		{
			ID:          "dead",
			Pool:        []string{"pool-0"},
			Dead:        true,
			RBEInstance: "rbe-0",
			Version:     "v2",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		"pool-0:DEAD_RBE:1",
		"pool-0:HYBRID:1",
		"pool-0:MAINTENANCE:1",
		"pool-0:QUARANTINED:1",
		"pool-0:RBE:3",
		"pool-0:SWARMING:1",
		"pool-1:HYBRID:1",
		"pool-1:RBE:3",
		"pool-1:SWARMING:1",
	}
	got := GlobalValues(t, mon.Cells, "swarming/rbe_migration/bots")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("swarming/rbe_migration/bots (-want +got):\n%s", diff)
	}

	want = []string{
		"dead:dead",
		"healthy-0:ready",
		"healthy-1:running",
		"healthy-2:ready",
		"healthy-3:ready",
		"healthy-4:ready",
		"healthy-5:ready",
		"maintenance:maintenance",
		"quarantined:quarantined",
	}
	got = PerBotValues(t, mon.Cells, "executors/status")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("executors/status (-want +got):\n%s", diff)
	}

	want = []string{
		"dead:pool:pool-0",
		"healthy-0:os:Ubuntu-22|pool:pool-0|pool:pool-1",
		"healthy-1:os:Ubuntu-22|pool:pool-0|pool:pool-1",
		"healthy-2:os:Ubuntu-22|pool:pool-0|pool:pool-1",
		"healthy-3:os:Ubuntu-22|pool:pool-0|pool:pool-1",
		"healthy-4:os:Ubuntu-22|pool:pool-0|pool:pool-1",
		"healthy-5:os:Ubuntu-22|pool:pool-0|pool:pool-1",
		"maintenance:pool:pool-0",
		"quarantined:pool:pool-0",
	}
	got = PerBotValues(t, mon.Cells, "executors/pool")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("executors/pool (-want +got):\n%s", diff)
	}

	want = []string{
		"dead:rbe-0",
		"healthy-0:rbe-0",
		"healthy-1:rbe-0",
		"healthy-2:rbe-0",
		"healthy-3:rbe-0",
		"healthy-4:rbe-1",
		"healthy-5:none",
		"maintenance:rbe-0",
		"quarantined:rbe-0",
	}
	got = PerBotValues(t, mon.Cells, "executors/rbe")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("executors/rbe (-want +got):\n%s", diff)
	}

	want = []string{
		"dead:v2",
		"healthy-0:v0",
		"healthy-1:v0",
		"healthy-2:v0",
		"healthy-3:v1",
		"healthy-4:v1",
		"healthy-5:v1",
		"maintenance:v2",
		"quarantined:v2",
	}
	got = PerBotValues(t, mon.Cells, "executors/version")
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("executors/version (-want +got):\n%s", diff)
	}
}

func TestPoolFromDimensions(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())

	mon := &monitor.Fake{
		CS: 10000, // send all values via one chunk
	}

	r := BotsMetricsReporter{
		ServiceName: "service",
		JobName:     "job",
		Monitor:     mon,
	}
	r.Prepare(ctx, 1, time.Time{})

	cases := []struct {
		input []string
		want  string
	}{
		{input: []string{}, want: ""},
		{
			input: []string{
				"bot_config:bot_config.py",
				"bot_size:e2-small",
				"cores:2",
				"cpu:x86",
				"cpu:x86-64",
				"cpu:x86-64-Broadwell_GCE",
				"cpu:x86-64-avx2",
				"gce:1",
				"gcp:chromeos-bot",
				"gpu:none",
				"os:Linux",
				"os:Ubuntu",
				"os:Ubuntu-22",
				"os:Ubuntu-22.04",
				"os:Ubuntu-22.04.1",
				"python:3",
				"python:3.8",
				"python:3.8.10+chromium.23",
			},
			want: "bot_config:bot_config.py|bot_size:e2-small|cores:2|cpu:x86-64-Broadwell_GCE|cpu:x86-64-avx2|gce:1|gcp:chromeos-bot|gpu:none|os:Linux|os:Ubuntu-22.04.1|python:3.8.10+chromium.23",
		},
		{
			input: []string{
				"bot_config:bot_config.py",
				"builder:android-angle-chromium-try",
				"caches:builder_5c1553edd9cb669432705d2201ae2e09effac3bb5a66c8316e03b0a828f6fca1_v2",
				"caches:git",
				"caches:goma_v2",
				"caches:vpython",
				"cores:8",
				"cpu:x86",
				"cpu:x86-64",
				"cpu:x86-64-Broadwell_GCE",
				"cpu:x86-64-avx2",
				"gce:1",
				"gcp:google.com:chromecompute",
				"gpu:none",
				"id:android-angle-chromium-try-0-ldw1",
				"image:chrome-jammy-23081300-d896075b897",
				"inside_docker:0",
				"kernel:6.2.0-26-generic",
				"kvm:1",
				"locale:en_US.UTF-8",
				"machine_type:n1-standard-8",
				"os:Linux",
				"os:Ubuntu",
				"os:Ubuntu-22",
				"os:Ubuntu-22.04",
				"os:Ubuntu-22.04.1",
				"pool:luci.chromium.try",
				"python:3",
				"python:3.8",
				"python:3.8.10+chromium.23",
				"server_version:7321-94edb82",
				"ssd:0",
				"zone:us",
				"zone:us-central",
				"zone:us-central1",
				"zone:us-central1-c",
			},
			want: "bot_config:bot_config.py|builder:android-angle-chromium-try|cores:8|cpu:x86-64-Broadwell_GCE|cpu:x86-64-avx2|gce:1|gcp:google.com:chromecompute|gpu:none|image:chrome-jammy-23081300-d896075b897|inside_docker:0|kernel:6.2.0-26-generic|kvm:1|locale:en_US.UTF-8|machine_type:n1-standard-8|os:Linux|os:Ubuntu-22.04.1|pool:luci.chromium.try|python:3.8.10+chromium.23|ssd:0|zone:us-central1-c",
		},
		{
			input: []string{
				"android_devices:1",
				"bot_config:browser_dut.py",
				"cipd_platform:linux-amd64",
				"device_abi:x86_64",
				"device_gms_core_version:25.22.34",
				"device_max_uid:<=18000",
				"device_os:13658449",
				"device_os:Baklava",
				"device_os:MAIN",
				"device_os_flavor:google",
				"device_os_type:userdebug",
				"device_os_version:16",
				"device_playstore_version:46.8.29-31",
				"device_type:brya",
				"display_attached:0",
				"drone:some-drone-name",
				"drone_image:some-drone-image",
				"drone_server:kube61-e",
				"dut_id:C254434",
				"dut_name:chrome-clank-chromeos8-row21-rack13-host5",
				"dut_state:ready",
				"gce:0",
				"hwid:some hwid",
				"id:cros-chrome-clank-chromeos8-row21-rack13-host5",
				"image:chrome-jammy-23081300-d896075b897",
				"inside_docker:0",
				"kernel:5.13.0-44-generic",
				"label-amt_manager_state:NOT_APPLICABLE",
				"label-arc:True",
				"label-audio_beamforming:none",
				"label-fingerprint_sensor:NONE",
				"label-gpu_id:intel:46a8",
				"label-gsc_chip:GSC_CHIP_H1",
				"label-hotwording:True",
				"label-hwid_sku:kano_12th_Gen_IntelR_CoreTM_i5_1235U_8GB",
				"label-internal_display:True",
				"label-lucidsleep:True",
				"label-model:kano",
				"label-multiduts:False",
				"label-os_type:OS_TYPE_CROS",
				"label-phase:PHASE_PVT",
				"label-platform:brya",
				"label-pool:chrome.tests",
				"label-pool-os:chrome.os",
				"label-servo_deployed:True",
				"label-servo_state:WORKING",
				"label-sku:131104",
				"label-storage:nvme",
				"label-stylus:True",
				"label-touchpad:True",
				"label-touchscreen:True",
				"label-variant:kano",
				"label-video_acceleration:VIDEO_ACCELERATION_ENC_H264",
				"label-wifi_chip:INTEL_GFP2_AX211",
				"locale:en_US.UTF-8",
				"os:Android",
				"pool:chrome",
				"python:3",
				"python:3.8",
				"python:3.8.0",
				"serial_number:some-serial-num",
				"temp_band:<30",
				"ufs_zone:ZONE_SFO36_OS",
				"version_info_os_type:ANDROID",
				"zone:cloud",
			},
			want: "bot_config:browser_dut.py|cipd_platform:linux-amd64|device_abi:x86_64|device_gms_core_version:25.22.34|device_max_uid:<=18000|device_os:13658449|device_os:Baklava|device_os:MAIN|device_os_flavor:google|device_os_type:userdebug|device_os_version:16|device_playstore_version:46.8.29-31|device_type:brya|display_attached:0|dut_state:ready|gce:0|image:chrome-jammy-23081300-d896075b897|inside_docker:0|kernel:5.13.0-44-generic|label-pool:chrome.tests|locale:en_US.UTF-8|os:Android|pool:chrome|python:3.8.0|ufs_zone:ZONE_SFO36_OS|version_info_os_type:ANDROID|zone:cloud",
		},
	}
	for i, c := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			got := r.shards[0].poolFromDimensions(c.input)
			//got := r.poolFromDimensions(c.input)
			if diff := cmp.Diff(c.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
