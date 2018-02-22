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

// Package backend implements configuration client backend interface and
// associated data types.
//
// The backend package implements a generalized implementation-oriented
// interface to configuration services that can be layered. Various backend
// implementations are best defined out of this package.
//
// All non-user-facing operations of cfgclient will operate on backend types.
package backend
