// Copyright 2022 The LUCI Authors.
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

// Package speepb contains Snoopy Policy Engine (SPEE)'s local gRPC server API
// definitions.
//
// These APIs allow policies (plugins) to report events to SPEE, where the
// events will be used for intrusion detection.
//
// network_proxy.proto: It's a copy-paste from BCID's google internal network
// proxy proto.
//
// TODO: Use Copybara for network_proxy.proto.
package speepb
