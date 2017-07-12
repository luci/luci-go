// Copyright 2016 The LUCI Authors.
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

// Package svcconfig stores service configuration for a LogDog instance.
//
// Each LogDog instantiation will have a single Config protobuf. It will be
// located under config set "services/<app-id>", path "services.cfg". The path
// is exposed via ServiceConfigFilename.
//
// Each LogDog project will have its own project-specific configuration. It will
// be located under config set "projects/<project-name>", path "<app-id>.cfg".
package svcconfig
