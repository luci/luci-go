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

package lucicfg

// Import all protos which we want to be available from the Starlark side.
// Starlark code relies on the protobuf registry in the interpreter process for
// type information.

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io/ioutil"
	"sync"

	proto_v1 "github.com/golang/protobuf/proto"
	descpb_v1 "github.com/golang/protobuf/protoc-gen-go/descriptor"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/proto"

	"go.chromium.org/luci/starlark/interpreter"
	"go.chromium.org/luci/starlark/protohacks"
	"go.chromium.org/luci/starlark/starlarkproto"

	_ "github.com/golang/protobuf/ptypes/any"
	_ "github.com/golang/protobuf/ptypes/duration"
	_ "github.com/golang/protobuf/ptypes/empty"
	_ "github.com/golang/protobuf/ptypes/struct"
	_ "github.com/golang/protobuf/ptypes/timestamp"
	_ "github.com/golang/protobuf/ptypes/wrappers"

	_ "google.golang.org/genproto/googleapis/type/dayofweek"

	_ "go.chromium.org/chromiumos/infra/proto/go/chromiumos"
	_ "go.chromium.org/chromiumos/infra/proto/go/device"
	_ "go.chromium.org/chromiumos/infra/proto/go/test_platform"
	_ "go.chromium.org/chromiumos/infra/proto/go/test_platform/config"
	_ "go.chromium.org/chromiumos/infra/proto/go/test_platform/migration/scheduler"
	_ "go.chromium.org/chromiumos/infra/proto/go/testplans"

	_ "go.chromium.org/luci/buildbucket/proto"
	_ "go.chromium.org/luci/common/proto/config"
	_ "go.chromium.org/luci/cq/api/config/v2"
	_ "go.chromium.org/luci/gce/api/config/v1"
	_ "go.chromium.org/luci/gce/api/projects/v1"
	_ "go.chromium.org/luci/logdog/api/config/svcconfig"
	_ "go.chromium.org/luci/luci_notify/api/config"
	_ "go.chromium.org/luci/milo/api/config"
	_ "go.chromium.org/luci/scheduler/appengine/messages"
	_ "go.chromium.org/luci/swarming/proto/config"
)

var noaliasCounter = 0

// Helper to generate unique keys for publicProtos.
//
// Eventually this whole map will be deleted, along with all these keys.
func noalias() string {
	noaliasCounter++
	return fmt.Sprintf("should_not_be_seen_ever_%d.proto", noaliasCounter)
}

// Mapping from a proto module path on the Starlark side to its proto package
// name and a path in the protobuf registry.
//
// TODO(vadimsh): Deprecated. Proto files should be referenced by their proto
// names. Entries marked with noalias() indicate proto files that are already
// imported using their proto names.
var publicProtos = map[string]struct {
	protoPkg  string
	protoPath string
}{
	// Buildbucket project config.
	//
	// load("@stdlib//internal/luci/proto.star", "buildbucket_pb")
	"luci/buildbucket/project_config.proto": {
		"buildbucket",
		"go.chromium.org/luci/buildbucket/proto/project_config.proto",
	},

	// "LUCI Config" project config.
	//
	// load("@stdlib//internal/luci/proto.star", "config_pb")
	"luci/config/project_config.proto": {
		"config",
		"go.chromium.org/luci/common/proto/config/project_config.proto",
	},

	// CrOS builder config.
	//
	// load(
	//     "@proto//external/cros/builder_config.proto",
	//     builder_config_pb="chromiumos")
	// load(
	//     "@proto//external/cros/common.proto",
	//     common_pb="chromiumos")
	"external/cros/builder_config.proto": {
		"chromiumos",
		"chromiumos/builder_config.proto",
	},
	"external/cros/common.proto": {
		"chromiumos",
		"chromiumos/common.proto",
	},

	// CrOS device config.
	//
	// load(
	//     "@proto//external/crosdevice/brand_id.proto",
	//     brand_id_pb="device")
	// load(
	//     "@proto//external/crosdevice/config_id.proto",
	//     config_id_pb="device")
	// load(
	//     "@proto//external/crosdevice/config.proto",
	//     config_pb="device")
	// load(
	//     "@proto//external/crosdevice/model_id.proto",
	//     model_id_pb="device")
	// load(
	//     "@proto//external/crosdevice/platform_id.proto",
	//     platform_id_pb="device")
	// load(
	//     "@proto//external/crosdevice/variant_id.proto",
	//     variant_id_pb="device")
	"external/crosdevice/brand_id.proto": {
		"device",
		"device/brand_id.proto",
	},
	"external/crosdevice/config_id.proto": {
		"device",
		"device/config_id.proto",
	},
	"external/crosdevice/config.proto": {
		"device",
		"device/config.proto",
	},
	"external/crosdevice/model_id.proto": {
		"device",
		"device/model_id.proto",
	},
	"external/crosdevice/platform_id.proto": {
		"device",
		"device/platform_id.proto",
	},
	"external/crosdevice/variant_id.proto": {
		"device",
		"device/variant_id.proto",
	},

	// CrOS testing project config.
	//
	// load(
	//     "@proto//external/crostesting/build_irrelevance_config.proto",
	//     build_irrelevance_config_pb="testplans")
	// load(
	//     "@proto//external/crostesting/common.proto",
	//     common_pb="testplans")
	// load(
	//     "@proto//external/crostesting/source_tree_test_config.proto",
	//     source_tree_test_config_pb="testplans")
	// load(
	//     "@proto//external/crostesting/target_test_requirements_config.proto",
	//     target_test_requirements_config_pb="testplans")
	// load(
	//     "@proto//external/crostesting/test_level_tweak.proto",
	//     test_level_tweak_pb="testplans")
	"external/crostesting/build_irrelevance_config.proto": {
		"testplans",
		"testplans/build_irrelevance_config.proto",
	},
	"external/crostesting/common.proto": {
		"testplans",
		"testplans/common.proto",
	},
	"external/crostesting/source_tree_test_config.proto": {
		"testplans",
		"testplans/source_tree_test_config.proto",
	},
	"external/crostesting/target_test_requirements_config.proto": {
		"testplans",
		"testplans/target_test_requirements_config.proto",
	},
	"external/crostesting/test_level_tweak.proto": {
		"testplans",
		"testplans/test_level_tweak.proto",
	},

	// CrOS test platform configuration.
	//
	// load(
	//     "@proto//external/cros/test_platform/request.proto",
	//     request_pb="test_platform")
	// load(
	//     "@proto//external/cros/test_platform/config/config.proto",
	//     config_pb = "test_platform.config")
	// load(
	//     "@proto//external/cros/test_platform/migration/scheduler/traffic_split.proto",
	//     migration_pb="test_platform.migration.scheduler")
	"external/cros/test_platform/request.proto": {
		"test_platform",
		"test_platform/request.proto",
	},
	"external/cros/test_platform/config/config.proto": {
		"test_platform.config",
		"test_platform/config/config.proto",
	},
	"external/cros/test_platform/migration/scheduler/traffic_split.proto": {
		"test_platform.migration.scheduler",
		"test_platform/migration/scheduler/traffic_split.proto",
	},

	// LogDog project config.
	//
	// load("@stdlib//internal/luci/proto.star", "logdog_pb")
	"luci/logdog/project_config.proto": {
		"svcconfig",
		"go.chromium.org/luci/logdog/api/config/svcconfig/project.proto",
	},

	// "LUCI Notify" project config.
	//
	// load("@stdlib//internal/luci/proto.star", "notify_pb")
	"luci/notify/project_config.proto": {
		"notify",
		"go.chromium.org/luci/luci_notify/api/config/notify.proto",
	},

	// Milo project config.
	//
	// load("@stdlib//internal/luci/proto.star", "milo_pb")
	"luci/milo/project_config.proto": {
		"milo",
		"go.chromium.org/luci/milo/api/config/project.proto",
	},

	// "LUCI Scheduler" project config.
	//
	// load("@stdlib//internal/luci/proto.star", "scheduler_pb")
	"luci/scheduler/project_config.proto": {
		"scheduler.config",
		"go.chromium.org/luci/scheduler/appengine/messages/config.proto",
	},

	// Commit Queue project configs (v2).
	//
	// load("@stdlib//internal/luci/proto.star", "cq_pb")
	"luci/cq/project_config.proto": {
		"cq.config",
		"go.chromium.org/luci/cq/api/config/v2/cq.proto",
	},

	// Swarming service configs.
	//
	// load("@proto//luci/swarming/bots.proto", bots_pb="swarming.config")
	// load("@proto//luci/swarming/config.proto", config_pb="swarming.config")
	// load("@proto//luci/swarming/pools.proto", pools_pb="swarming.config")
	"luci/swarming/bots.proto": {
		"swarming.config",
		"go.chromium.org/luci/swarming/proto/config/bots.proto",
	},
	"luci/swarming/config.proto": {
		"swarming.config",
		"go.chromium.org/luci/swarming/proto/config/config.proto",
	},
	"luci/swarming/pools.proto": {
		"swarming.config",
		"go.chromium.org/luci/swarming/proto/config/pools.proto",
	},

	// GCE Provider service configs.
	//
	// load("@proto//luci/gce/config.proto", config_pb="config")
	// load("@proto//luci/gce/projects.proto", projects_pb="projects")
	"luci/gce/config.proto": {
		"config",
		"go.chromium.org/luci/gce/api/config/v1/config.proto",
	},
	"luci/gce/projects.proto": {
		"projects",
		"go.chromium.org/luci/gce/api/projects/v1/config.proto",
	},

	// Various well-known proto types.
	//
	// load("@proto//google/protobuf/any.proto", any_pb="google.protobuf")
	// load("@proto//google/protobuf/duration.proto", duration_pb="google.protobuf")
	// load("@proto//google/protobuf/empty.proto", empty_pb="google.protobuf")
	// load("@proto//google/protobuf/struct.proto", struct_pb="google.protobuf")
	// load("@proto//google/protobuf/timestamp.proto", timestamp_pb="google.protobuf")
	// load("@proto//google/protobuf/wrappers.proto", wrappers_pb="google.protobuf")
	noalias(): {"google.protobuf", "google/protobuf/any.proto"},
	noalias(): {"google.protobuf", "google/protobuf/duration.proto"},
	noalias(): {"google.protobuf", "google/protobuf/empty.proto"},
	noalias(): {"google.protobuf", "google/protobuf/struct.proto"},
	noalias(): {"google.protobuf", "google/protobuf/timestamp.proto"},
	noalias(): {"google.protobuf", "google/protobuf/wrappers.proto"},

	// Extension proto types.
	//
	// load("@proto//google/type/dayofweek.proto", dayofweek_pb="google.type")
	noalias(): {"google.type", "google/type/dayofweek.proto"},
}

// Note: we use sync.Once instead of init() to allow tests to add stuff to
// publicProtos in their own init().
var (
	loader interpreter.Loader
	once   sync.Once
)

// protoLoader returns a loader that is capable of loading publicProtos.
func protoLoader() interpreter.Loader {
	once.Do(func() {
		ploader := starlarkproto.NewLoader()

		// Populate protobuf v2 loader using protobuf v1 registry embedded into
		// the binary. This visits imports in topological order, to make sure all
		// cross-file references are correctly resolved. We assume there are no
		// circular dependencies (if there are, they'll be caught by hanging unit
		// tests).
		visited := stringset.New(0)
		for _, info := range publicProtos {
			if err := addWithDeps(ploader, info.protoPath, visited); err != nil {
				panic(fmt.Errorf("%s: %s", info.protoPath, err))
			}
		}

		// Use this loader to resolve load("@proto://...") references.
		loader = func(path string) (dict starlark.StringDict, _ string, err error) {
			// First try to load the proto using its real proto path. Otherwise
			// assume 'path' as in alias in publicProtos map.
			//
			// TODO(vadimsh): Remove aliases once all scripts are migrated to use
			// real proto paths.
			mod, err := ploader.Module(path)
			if err != nil {
				info, ok := publicProtos[path]
				if !ok {
					return nil, "", errors.New("no such proto file in lucicfg's internal proto registry")
				}
				mod, err = ploader.Module(info.protoPath)
			}
			if err != nil {
				return nil, "", err
			}
			return starlark.StringDict{mod.Name: mod}, "", nil
		}
	})
	return loader
}

// addWithDeps loads dependencies of 'path', and then 'path' itself.
func addWithDeps(l *starlarkproto.Loader, path string, visited stringset.Set) error {
	if !visited.Add(path) {
		return nil // visited it already
	}
	raw, deps, err := rawFileDescriptor(path)
	if err != nil {
		return err
	}
	for _, d := range deps {
		if err := addWithDeps(l, d, visited); err != nil {
			return fmt.Errorf("%s: %s", d, err)
		}
	}
	return l.AddDescriptor(raw)
}

// rawFileDescriptor extracts raw FileDescriptor from protobuf v1 registry and
// returns it along with a list of *.proto files it depends on (imports).
func rawFileDescriptor(name string) ([]byte, []string, error) {
	gzblob := proto_v1.FileDescriptor(name)
	if gzblob == nil {
		return nil, nil, fmt.Errorf("proto %q is not in the registry", name)
	}

	r, err := gzip.NewReader(bytes.NewReader(gzblob))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open gzip reader for %q - %s", name, err)
	}
	defer r.Close()

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to uncompress descriptor for %q - %s", name, err)
	}

	fd := &descpb_v1.FileDescriptorProto{}
	if err := proto_v1.Unmarshal(b, fd); err != nil {
		return nil, nil, fmt.Errorf("malformed FileDescriptorProto %q - %s", name, err)
	}
	return b, fd.Dependency, nil
}

// protoMessageDoc returns the message name and a link to its schema doc.
//
// If there's no documentation, returns two empty strings.
func protoMessageDoc(msg *starlarkproto.Message) (name, doc string) {
	fd := msg.MessageType().Descriptor().ParentFile()
	if fd == nil {
		return "", ""
	}
	// Try to grab doc_url from `option (lucicfg.file_metadata) = {...}` embedded
	// into the proto descriptor. Since we still use proto v1 as our *.go code
	// generator, but proto v2 as our runtime API, there are some interoperability
	// issues we solve by round-tripping descriptorpb.FileOptions message through
	// proto serialization (we serialize it as v2 proto, and deserialize it as v1
	// proto, which allows us to use code-generated proto extensions API).
	//
	// If something fails, just give up, it's not a crucial functionality. Note
	// that unit tests verify the golden path.
	if blob, _ := protohacks.FileOptions(fd); blob != nil {
		// TODO(vadimsh): Move this to common/proto.
		fileOpts := &descpb_v1.FileOptions{}
		if err := proto_v1.Unmarshal(blob, fileOpts); err == nil {
			exts, err := proto_v1.GetExtensions(fileOpts, []*proto_v1.ExtensionDesc{proto.E_FileMetadata})
			if err == nil && exts[0] != nil {
				if meta := exts[0].(*proto.Metadata); meta.GetDocUrl() != "" {
					return string(msg.MessageType().Descriptor().Name()), meta.GetDocUrl()
				}
			}
		}
	}
	return "", "" // not a public proto
}
