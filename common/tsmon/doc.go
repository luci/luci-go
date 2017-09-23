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

// Package tsmon contains global state and utility functions for configuring
// and interacting with tsmon. This has a similar API to the infra-python
// ts_mon module.
//
// If your application accepts command line flags, then call InitializeFromFlags
// from your main function.
//
// If you use tsmon on AppEngine, then see appengine/tsmon package doc.
package tsmon
