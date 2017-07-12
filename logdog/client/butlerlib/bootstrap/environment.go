// Copyright 2015 The LUCI Authors.
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

package bootstrap

// Environment variable names
const (
	// EnvStreamServerPath is the path to the Butler's stream server endpoint.
	//
	// This can be used by applications to initiate a new Butler stream with an
	// existing Butler stream server process. If a subprocess is launched with a
	// stream server configuration, it will propagate this path to its child
	// processes.
	EnvStreamServerPath = "LOGDOG_STREAM_SERVER_PATH"

	// EnvStreamProject is the environment variable set to the configured stream
	// project name.
	EnvStreamProject = "LOGDOG_STREAM_PROJECT"

	// EnvStreamPrefix is the environment variable set to the configured
	// stream name prefix.
	EnvStreamPrefix = "LOGDOG_STREAM_PREFIX"

	// EnvCoordinatorHost is the environment variable set to the host name of
	// the upstream Coordinator service.
	EnvCoordinatorHost = "LOGDOG_COORDINATOR_HOST"
)
