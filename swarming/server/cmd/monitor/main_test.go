// Copyright 2023 The LUCI Authors.
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

package main

import (
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPoolFromDimensions(t *testing.T) {
	t.Parallel()

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
	}
	for i, c := range cases {
		i := i
		c := c
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			got := poolFromDimensions(c.input)
			if diff := cmp.Diff(c.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
