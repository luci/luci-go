// Copyright 2021 The LUCI Authors.
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

// Package cron allows to register handlers called by Cloud Scheduler.
//
// The HTTP endpoints exposed by this module perform necessary authorization
// checks and route requests to registered handlers, collecting monitoring
// metrics from them.
//
// Note that you still need to configure Cloud Scheduler jobs (or cron.yaml when
// running on Appengine). By default registered handlers are exposed as
// "/internal/cron/<handler-id>" endpoints. This URL path should be used when
// configuring Cloud Scheduler jobs or in cron.yaml when running on Appengine.
package cron
