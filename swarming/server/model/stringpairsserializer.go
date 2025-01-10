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
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// StringPair is a key-value pair of strings.
type StringPair struct {
	Key   string
	Value string
}

type stringPairsSerializer struct {
	// current is the current field the serializer is processing.
	current string
	// pairs contain the already processed fields.
	pairs []StringPair
}

// enter appends field to s.current
func (s *stringPairsSerializer) enter(field string) {
	if field == "" {
		panic("field cannot be empty")
	}
	if strings.Contains(field, ".") {
		panic(fmt.Sprintf("field cannot contain a dot: %q", field))
	}
	if s.current == "" {
		s.current = field
		return
	}
	s.current = s.current + "." + field
}

// exit pops the last element of s.current.
func (s *stringPairsSerializer) exit() {
	if s.current == "" {
		return
	}

	lastIndex := strings.LastIndex(s.current, ".")
	if lastIndex == -1 {
		s.current = ""
		return
	}
	s.current = s.current[:lastIndex]
}

func (s *stringPairsSerializer) writeString(subKey, val string) {
	var key string
	if s.current == "" {
		key = subKey
	} else {
		key = s.current + "." + subKey
	}
	s.pairs = append(s.pairs, StringPair{Key: key, Value: val})
}

func (s *stringPairsSerializer) writeBool(key string, b bool) {
	if b {
		s.writeString(key, "true")
	} else {
		s.writeString(key, "false")
	}
}

func (s *stringPairsSerializer) writeInt64(key string, i int64) {
	s.writeString(key, strconv.FormatInt(i, 10))
}

func (s *stringPairsSerializer) writeStringSlice(key string, slice []string, shouldSort bool) {
	s.enter(key)
	if shouldSort {
		sort.Strings(slice)
	}
	for i, val := range slice {
		s.writeString(fmt.Sprintf("%d", i), val)
	}
	s.exit()
}

func (s *stringPairsSerializer) writeEnv(env Env) {
	s.enter("env")
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		s.enter(fmt.Sprintf("%d", i))
		s.writeString("key", k)
		s.writeString("value", env[k])
		s.exit()
	}
	s.exit()
}

func (s *stringPairsSerializer) writeEnvPrefixes(prefixes EnvPrefixes) {
	s.enter("env_prefixes")
	keys := make([]string, 0, len(prefixes))
	for k := range prefixes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		s.enter(fmt.Sprintf("%d", i))
		s.writeString("key", k)
		s.writeStringSlice("value", prefixes[k], false)
		s.exit()
	}
	s.exit()
}

func (s *stringPairsSerializer) writeTaskDimensions(dimensions TaskDimensions) {
	s.enter("dimensions")
	keys := make([]string, 0, len(dimensions))
	for k := range dimensions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		s.enter(fmt.Sprintf("%d", i))
		s.writeString("key", k)
		s.writeStringSlice("value", dimensions[k], true)
		s.exit()
	}
	s.exit()
}

func (s *stringPairsSerializer) writeCacheEntries(caches []CacheEntry) {
	s.enter("caches")
	// Sort caches by Name then by Path
	sort.Slice(caches, func(i, j int) bool {
		if caches[i].Name != caches[j].Name {
			return caches[i].Name < caches[j].Name
		}
		return caches[i].Path < caches[j].Path
	})

	for i, cache := range caches {
		s.enter(fmt.Sprintf("%d", i))
		s.writeString("name", cache.Name)
		s.writeString("path", cache.Path)
		s.exit()
	}
	s.exit()
}

func (s *stringPairsSerializer) writeCASReference(cas CASReference) {
	if (cas == CASReference{}) {
		return
	}
	s.enter("cas_input_root")
	s.writeString("cas_instance", cas.CASInstance)
	s.writeCASDigest(cas.Digest)
	s.exit()
}

func (s *stringPairsSerializer) writeCASDigest(digest CASDigest) {
	if (digest == CASDigest{}) {
		return
	}
	s.enter("digest")
	s.writeString("hash", digest.Hash)
	s.writeInt64("size_bytes", digest.SizeBytes)
	s.exit()
}

func (s *stringPairsSerializer) writeCIPDInput(cipd CIPDInput) {
	if reflect.DeepEqual(cipd, CIPDInput{}) {
		return
	}
	s.enter("cipd_input")
	s.writeString("server", cipd.Server)
	s.writeCIPDPackage("client_package", cipd.ClientPackage)
	s.writeCIPDPackages("packages", cipd.Packages)
	s.exit()
}

func (s *stringPairsSerializer) writeCIPDPackage(key string, pkg CIPDPackage) {
	if (pkg == CIPDPackage{}) {
		return
	}
	s.enter(key)
	s.writeString("package_name", pkg.PackageName)
	s.writeString("version", pkg.Version)
	s.writeString("path", pkg.Path)
	s.exit()
}

func (s *stringPairsSerializer) writeCIPDPackages(key string, packages []CIPDPackage) {
	s.enter(key)
	// Sort CIPDPackage by PackageName, then Version, then Path.
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].PackageName != packages[j].PackageName {
			return packages[i].PackageName < packages[j].PackageName
		}
		if packages[i].Version != packages[j].Version {
			return packages[i].Version < packages[j].Version
		}
		return packages[i].Path < packages[j].Path
	})
	for i, pkg := range packages {
		s.writeCIPDPackage(fmt.Sprintf("%d", i), pkg)
	}
	s.exit()
}

func (s *stringPairsSerializer) writeContainment(containment Containment) {
	if (containment == Containment{}) {
		return
	}
	s.enter("containment")
	s.writeInt64("containment_type", int64(containment.ContainmentType))
	s.writeBool("lower_priority", containment.LowerPriority)
	s.writeInt64("limit_processes", containment.LimitProcesses)
	s.writeInt64("limit_total_committed_memory", containment.LimitTotalCommittedMemory)
	s.exit()
}

func (s *stringPairsSerializer) writeTaskProperties(props TaskProperties) {
	if reflect.DeepEqual(props, TaskProperties{}) {
		return
	}
	// Handle basic types and struct fields
	s.writeBool("idempotent", props.Idempotent)
	s.writeString("relativeCwd", props.RelativeCwd)
	s.writeInt64("execution_timeout_secs", props.ExecutionTimeoutSecs)
	s.writeInt64("grace_period_secs", props.GracePeriodSecs)
	s.writeInt64("io_timeout_secs", props.IOTimeoutSecs)

	// Handle slices (command, outputs)
	s.writeStringSlice("command", props.Command, false)
	s.writeStringSlice("outputs", props.Outputs, true)

	// Handle maps (env, env_prefixes, dimensions)
	s.writeEnv(props.Env)
	s.writeEnvPrefixes(props.EnvPrefixes)
	s.writeTaskDimensions(props.Dimensions)

	// Handle nested structs
	s.writeCacheEntries(props.Caches)
	s.writeCASReference(props.CASInputRoot)
	s.writeCIPDInput(props.CIPDInput)
	s.writeContainment(props.Containment)
}

func (s *stringPairsSerializer) toBytes(props TaskProperties, sb *SecretBytes) ([]byte, error) {
	s.writeTaskProperties(props)
	if sb != nil {
		s.writeString("secret_bytes", string(sb.SecretBytes))
	}

	buf := &bytes.Buffer{}
	for _, pair := range s.pairs {
		if err := binary.Write(buf, binary.LittleEndian, int32(len(pair.Key))); err != nil {
			panic(err)
		}
		buf.WriteString(pair.Key)
		if err := binary.Write(buf, binary.LittleEndian, int32(len(pair.Value))); err != nil {
			panic(err)
		}
		buf.WriteString(pair.Value)
	}

	return buf.Bytes(), nil
}
