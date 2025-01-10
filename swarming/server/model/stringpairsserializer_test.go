// Copyright 2025 The LUCI Authors.
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
	"time"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStringPairsSerializer(t *testing.T) {
	t.Parallel()
	testTime := time.Date(2025, time.January, 1, 2, 3, 4, 0, time.UTC)
	slice := createTaskSlice("a", testTime, testTime.Add(10*time.Minute), "pool", "botID")
	t.Run("serialize_properties", func(t *testing.T) {
		s := &stringPairsSerializer{}
		s.writeTaskProperties(slice.Properties)
		expected := []StringPair{
			{Key: "idempotent", Value: "true"},
			{Key: "relativeCwd", Value: "./rel/cwd"},
			{Key: "execution_timeout_secs", Value: "123"},
			{Key: "grace_period_secs", Value: "456"},
			{Key: "io_timeout_secs", Value: "789"},
			{Key: "command.0", Value: "run"},
			{Key: "command.1", Value: "a"},
			{Key: "outputs.0", Value: "o1"},
			{Key: "outputs.1", Value: "o2"},
			{Key: "env.0.key", Value: "k1"},
			{Key: "env.0.value", Value: "v1"},
			{Key: "env.1.key", Value: "k2"},
			{Key: "env.1.value", Value: "a"},
			{Key: "env_prefixes.0.key", Value: "p1"},
			{Key: "env_prefixes.0.value.0", Value: "v1"},
			{Key: "env_prefixes.0.value.1", Value: "v2"},
			{Key: "env_prefixes.1.key", Value: "p2"},
			{Key: "env_prefixes.1.value.0", Value: "a"},
			{Key: "dimensions.0.key", Value: "d1"},
			{Key: "dimensions.0.value.0", Value: "v1"},
			{Key: "dimensions.0.value.1", Value: "v2"},
			{Key: "dimensions.1.key", Value: "d2"},
			{Key: "dimensions.1.value.0", Value: "a"},
			{Key: "dimensions.2.key", Value: "id"},
			{Key: "dimensions.2.value.0", Value: "botID"},
			{Key: "dimensions.3.key", Value: "pool"},
			{Key: "dimensions.3.value.0", Value: "pool"},
			{Key: "caches.0.name", Value: "n1"},
			{Key: "caches.0.path", Value: "p1"},
			{Key: "caches.1.name", Value: "n2"},
			{Key: "caches.1.path", Value: "p2"},
			{Key: "cas_input_root.cas_instance", Value: "cas-inst"},
			{Key: "cas_input_root.digest.hash", Value: "cas-hash"},
			{Key: "cas_input_root.digest.size_bytes", Value: "1234"},
			{Key: "cipd_input.server", Value: "server"},
			{Key: "cipd_input.client_package.package_name", Value: "client-package"},
			{Key: "cipd_input.client_package.version", Value: "client-version"},
			{Key: "cipd_input.client_package.path", Value: ""},
			{Key: "cipd_input.packages.0.package_name", Value: "pkg1"},
			{Key: "cipd_input.packages.0.version", Value: "ver1"},
			{Key: "cipd_input.packages.0.path", Value: "path1"},
			{Key: "cipd_input.packages.1.package_name", Value: "pkg2"},
			{Key: "cipd_input.packages.1.version", Value: "ver2"},
			{Key: "cipd_input.packages.1.path", Value: "path2"},
			{Key: "containment.containment_type", Value: "123"},
			{Key: "containment.lower_priority", Value: "true"},
			{Key: "containment.limit_processes", Value: "456"},
			{Key: "containment.limit_total_committed_memory", Value: "789"},
		}
		assert.That(t, s.pairs, should.Match(expected))
	})

	t.Run("serialize_properties_empty", func(t *testing.T) {
		s := &stringPairsSerializer{}
		s.writeTaskProperties(TaskProperties{})
		assert.Loosely(t, s.pairs, should.BeEmpty)
	})

	t.Run("serialize_properties_partial", func(t *testing.T) {
		s := &stringPairsSerializer{}
		s.writeTaskProperties(TaskProperties{
			EnvPrefixes: EnvPrefixes{
				"p1": {"v2", "v1"},
			},
			Dimensions: TaskDimensions{
				"d1": {"v1", "v2"},
			},
		})
		expected := []StringPair{
			{Key: "idempotent", Value: "false"},
			{Key: "relativeCwd", Value: ""},
			{Key: "execution_timeout_secs", Value: "0"},
			{Key: "grace_period_secs", Value: "0"},
			{Key: "io_timeout_secs", Value: "0"},
			{Key: "env_prefixes.0.key", Value: "p1"},
			{Key: "env_prefixes.0.value.0", Value: "v2"},
			{Key: "env_prefixes.0.value.1", Value: "v1"},
			{Key: "dimensions.0.key", Value: "d1"},
			{Key: "dimensions.0.value.0", Value: "v1"},
			{Key: "dimensions.0.value.1", Value: "v2"},
		}

		assert.That(t, s.pairs, should.Match(expected))
	})
}
