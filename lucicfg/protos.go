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
	"github.com/golang/protobuf/descriptor"
	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/starlark/interpreter"

	_ "github.com/golang/protobuf/ptypes/any"
	_ "github.com/golang/protobuf/ptypes/duration"
	_ "github.com/golang/protobuf/ptypes/empty"
	_ "github.com/golang/protobuf/ptypes/struct"
	_ "github.com/golang/protobuf/ptypes/timestamp"
	_ "github.com/golang/protobuf/ptypes/wrappers"

	_ "google.golang.org/genproto/googleapis/type/dayofweek"

	_ "go.chromium.org/chromiumos/infra/proto/go/chromiumos"
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

// Mapping from a proto module path on the Starlark side to its proto package
// name, a path within GOPATH, and an optional link to schema docs.
//
// Go proto location can be changed without altering Starlark API. Proto package
// name is still important and should not be changed. It is here mostly for the
// documentation purpose.
//
// There's also a test that asserts real proto packages (per generated *.pb2.go)
// match what's specified here.
var publicProtos = map[string]struct {
	protoPkg string
	goPath   string
	docURL   string
}{
	// Buildbucket project config.
	//
	// load("@proto//luci/buildbucket/project_config.proto", buildbucket_pb="buildbucket")
	"luci/buildbucket/project_config.proto": {
		"buildbucket",
		"go.chromium.org/luci/buildbucket/proto/project_config.proto",
		"https://luci-config.appspot.com/schemas/projects:buildbucket.cfg",
	},

	// "LUCI Config" project config.
	//
	// load("@proto//luci/config/project_config.proto", config_pb="config")
	"luci/config/project_config.proto": {
		"config",
		"go.chromium.org/luci/common/proto/config/project_config.proto",
		"https://luci-config.appspot.com/schemas/projects:project.cfg",
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
		"",
	},
	"external/cros/common.proto": {
		"chromiumos",
		"chromiumos/common.proto",
		"",
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
	"external/crostesting/build_irrelevance_config.proto": {
		"testplans",
		"testplans/build_irrelevance_config.proto",
		"",
	},
	"external/crostesting/common.proto": {
		"testplans",
		"testplans/common.proto",
		"",
	},
	"external/crostesting/source_tree_test_config.proto": {
		"testplans",
		"testplans/source_tree_test_config.proto",
		"",
	},
	"external/crostesting/target_test_requirements_config.proto": {
		"testplans",
		"testplans/target_test_requirements_config.proto",
		"",
	},

	// LogDog project config.
	//
	// load("@proto//luci/logdog/project_config.proto", logdog_pb="svcconfig")
	"luci/logdog/project_config.proto": {
		"svcconfig",
		"go.chromium.org/luci/logdog/api/config/svcconfig/project.proto",
		"https://luci-config.appspot.com/schemas/projects:luci-logdog.cfg",
	},

	// "LUCI Notify" project config.
	//
	// load("@proto//luci/notify/project_config.proto", notify_pb="notify")
	"luci/notify/project_config.proto": {
		"notify",
		"go.chromium.org/luci/luci_notify/api/config/notify.proto",
		"https://luci-config.appspot.com/schemas/projects:luci-notify.cfg",
	},

	// Milo project config.
	//
	// load("@proto//luci/milo/project_config.proto", milo_pb="milo")
	"luci/milo/project_config.proto": {
		"milo",
		"go.chromium.org/luci/milo/api/config/project.proto",
		"https://luci-config.appspot.com/schemas/projects:luci-milo.cfg",
	},

	// "LUCI Scheduler" project config.
	//
	// load("@proto//luci/scheduler/project_config.proto", scheduler_pb="scheduler.config")
	"luci/scheduler/project_config.proto": {
		"scheduler.config",
		"go.chromium.org/luci/scheduler/appengine/messages/config.proto",
		"https://luci-config.appspot.com/schemas/projects:luci-scheduler.cfg",
	},

	// Commit Queue project configs (v2).
	//
	// load("@proto//luci/cq/project_config.proto", cq_pb="cq.config")
	"luci/cq/project_config.proto": {
		"cq.config",
		"go.chromium.org/luci/cq/api/config/v2/cq.proto",
		"https://luci-config.appspot.com/schemas/projects:commit-queue.cfg",
	},

	// Swarming service configs.
	//
	// load("@proto//luci/swarming/bots.proto", bots_pb="swarming")
	// load("@proto//luci/swarming/config.proto", config_pb="swarming")
	// load("@proto//luci/swarming/pools.proto", pools_pb="swarming")
	"luci/swarming/bots.proto": {
		"swarming",
		"go.chromium.org/luci/swarming/proto/config/bots.proto",
		"https://luci-config.appspot.com/schemas/services/swarming:bots.cfg",
	},
	"luci/swarming/config.proto": {
		"swarming",
		"go.chromium.org/luci/swarming/proto/config/config.proto",
		"https://luci-config.appspot.com/schemas/services/swarming:settings.cfg",
	},
	"luci/swarming/pools.proto": {
		"swarming",
		"go.chromium.org/luci/swarming/proto/config/pools.proto",
		"https://luci-config.appspot.com/schemas/services/swarming:pools.cfg",
	},

	// GCE Provider service configs.
	//
	// load("@proto//luci/gce/config.proto", config_pb="config")
	// load("@proto//luci/gce/projects.proto", projects_pb="projects")
	"luci/gce/config.proto": {
		"config",
		"go.chromium.org/luci/gce/api/config/v1/config.proto",
		"https://luci-config.appspot.com/schemas/services/gce-provider:vms.cfg",
	},
	"luci/gce/projects.proto": {
		"projects",
		"go.chromium.org/luci/gce/api/projects/v1/config.proto",
		"https://luci-config.appspot.com/schemas/services/gce-provider:projects.cfg",
	},

	// Various well-known proto types.
	//
	// load("@proto//google/protobuf/any.proto", any_pb="google.protobuf")
	// load("@proto//google/protobuf/duration.proto", duration_pb="google.protobuf")
	// load("@proto//google/protobuf/empty.proto", empty_pb="google.protobuf")
	// load("@proto//google/protobuf/struct.proto", struct_pb="google.protobuf")
	// load("@proto//google/protobuf/timestamp.proto", timestamp_pb="google.protobuf")
	// load("@proto//google/protobuf/wrappers.proto", wrappers_pb="google.protobuf")
	"google/protobuf/any.proto":       {"google.protobuf", "google/protobuf/any.proto", ""},
	"google/protobuf/duration.proto":  {"google.protobuf", "google/protobuf/duration.proto", ""},
	"google/protobuf/empty.proto":     {"google.protobuf", "google/protobuf/empty.proto", ""},
	"google/protobuf/struct.proto":    {"google.protobuf", "google/protobuf/struct.proto", ""},
	"google/protobuf/timestamp.proto": {"google.protobuf", "google/protobuf/timestamp.proto", ""},
	"google/protobuf/wrappers.proto":  {"google.protobuf", "google/protobuf/wrappers.proto", ""},

	// Extension proto types.
	//
	// load("@proto//google/type/dayofweek.proto", dayofweek_pb="google.type")
	"google/type/dayofweek.proto": {"google.type", "google/type/dayofweek.proto", ""},
}

// protoLoader returns a loader that is capable of loading publicProtos.
func protoLoader() interpreter.Loader {
	mapping := make(map[string]string, len(publicProtos))
	for path, proto := range publicProtos {
		mapping[path] = proto.goPath
	}
	return interpreter.ProtoLoader(mapping)
}

// protoMessageDoc returns the message name and a link to its schema doc.
//
// If there's no documentation, returns two empty strings.
func protoMessageDoc(msg proto.Message) (name, doc string) {
	withDesc, ok := msg.(descriptor.Message)
	if !ok {
		return "", ""
	}

	fd, md := descriptor.ForMessage(withDesc)
	if fd == nil || md == nil {
		return "", ""
	}

	for _, info := range publicProtos {
		// Find the exact same *.proto file within publicProtos struct.
		if info.goPath == fd.GetName() {
			if info.docURL == "" {
				return "", "" // no docs for it
			}
			return md.GetName(), info.docURL
		}
	}

	return "", "" // not a public proto
}
