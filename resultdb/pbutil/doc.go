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

// Package pbutil implements utility functions for ResultDB protobuf messages.
//
// TODO(nodir): many of these functions are not hardly useful for clients, e.g.
// ones involved in request validation. The request validation functions
// themselves are not in this package. Ideally all symbols in this package
// are useful for others. Otherwise pbutil should be in internal/.
package pbutil
