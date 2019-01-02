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
	"go.chromium.org/luci/starlark/interpreter"

	_ "github.com/golang/protobuf/ptypes/any"
	_ "github.com/golang/protobuf/ptypes/duration"
	_ "github.com/golang/protobuf/ptypes/empty"
	_ "github.com/golang/protobuf/ptypes/struct"
	_ "github.com/golang/protobuf/ptypes/timestamp"
	_ "github.com/golang/protobuf/ptypes/wrappers"

	_ "go.chromium.org/luci/buildbucket/proto/config"
	_ "go.chromium.org/luci/common/proto/config"
	_ "go.chromium.org/luci/logdog/api/config/svcconfig"
	_ "go.chromium.org/luci/luci_notify/api/config"
	_ "go.chromium.org/luci/milo/api/config"
	_ "go.chromium.org/luci/scheduler/appengine/messages"
)

// Mapping from a proto module path on the Starlark side to its proto package
// name and path within luci-go repository.
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
}{
	// Buildbucket project config.
	//
	// load("@proto//luci/buildbucket/project_config.proto", buildbucket_pb="buildbucket")
	"luci/buildbucket/project_config.proto": {
		"buildbucket",
		"go.chromium.org/luci/buildbucket/proto/config/project_config.proto",
	},

	// "LUCI Config" project config.
	//
	// load("@proto//luci/config/project_config.proto", config_pb="config")
	"luci/config/project_config.proto": {
		"config",
		"go.chromium.org/luci/common/proto/config/project_config.proto",
	},

	// LogDog project config.
	//
	// load("@proto//luci/logdog/project_config.proto", logdog_pb="svcconfig")
	"luci/logdog/project_config.proto": {
		"svcconfig",
		"go.chromium.org/luci/logdog/api/config/svcconfig/project.proto",
	},

	// "LUCI Notify" project config.
	//
	// load("@proto//luci/notify/project_config.proto", notify_pb="notify")
	"luci/notify/project_config.proto": {
		"notify",
		"go.chromium.org/luci/luci_notify/api/config/notify.proto",
	},

	// Milo project config.
	//
	// load("@proto//luci/milo/project_config.proto", milo_pb="milo")
	"luci/milo/project_config.proto": {
		"milo",
		"go.chromium.org/luci/milo/api/config/project.proto",
	},

	// "LUCI Scheduler" project config.
	//
	// load("@proto//luci/scheduler/project_config.proto", scheduler_pb="scheduler.config")
	"luci/scheduler/project_config.proto": {
		"scheduler.config",
		"go.chromium.org/luci/scheduler/appengine/messages/config.proto",
	},

	// Various well-known proto types.
	//
	// load("@proto//google/protobuf/any.proto", any_pb="google.protobuf")
	// load("@proto//google/protobuf/duration.proto", duration_pb="google.protobuf")
	// load("@proto//google/protobuf/empty.proto", empty_pb="google.protobuf")
	// load("@proto//google/protobuf/struct.proto", struct_pb="google.protobuf")
	// load("@proto//google/protobuf/timestamp.proto", timestamp_pb="google.protobuf")
	// load("@proto//google/protobuf/wrappers.proto", wrappers_pb="google.protobuf")
	"google/protobuf/any.proto":       {"google.protobuf", "google/protobuf/any.proto"},
	"google/protobuf/duration.proto":  {"google.protobuf", "google/protobuf/duration.proto"},
	"google/protobuf/empty.proto":     {"google.protobuf", "google/protobuf/empty.proto"},
	"google/protobuf/struct.proto":    {"google.protobuf", "google/protobuf/struct.proto"},
	"google/protobuf/timestamp.proto": {"google.protobuf", "google/protobuf/timestamp.proto"},
	"google/protobuf/wrappers.proto":  {"google.protobuf", "google/protobuf/wrappers.proto"},
}

// protoLoader returns a loader that is capable of loading publicProtos.
func protoLoader() interpreter.Loader {
	mapping := make(map[string]string, len(publicProtos))
	for path, proto := range publicProtos {
		mapping[path] = proto.goPath
	}
	return interpreter.ProtoLoader(mapping)
}
