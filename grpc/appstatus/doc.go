// Copyright 2019 The LUCI Authors.
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

// Package appstatus can attach/reterieve an application-specific response
// status to/from an error. It designed to prevent accidental exposure of
// internal statuses to RPC clients, for example Spanner's statuses.
//
// TODO(nodir): move the package to luci/grpc if this approach proves useful.
package appstatus
