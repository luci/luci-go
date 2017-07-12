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

// Package config implements common LogDog daemon configuration.
//
// The majority of LogDog configuration is loaded from "luci-config". The
// specific "luci-config" instance is passed to the daemon by the LogDog
// Coordaintor service. This library implements the logic to pull the
// configuration paramerers, load the "luci-config" configuration, and expose it
// to the service daemon.
//
// Additionally, for testing, an option is implemented that allows the user to
// read the configuration from a local file (as a text protobuf) instead of
// from the Coordinator.
package config
