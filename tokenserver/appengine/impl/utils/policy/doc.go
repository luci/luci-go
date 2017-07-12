// Copyright 2017 The LUCI Authors.
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

// Package policy contains implementation of Policy parsing and querying.
//
// Policies are configs fetched from LUCI Config and used from Token Server RPCs
// for various decisions. Each policy is defined by one or more config files,
// all fetched at a single revision (for consistency) and cached in single
// datastore entity (for faster fetches).
//
// The defining properties of policy configs are:
//  * They are global (i.e they are service configs, not per-project ones).
//  * The content is mostly static.
//  * They are queried from performance critical RPC handlers.
//
// This suggests to heavily cache policies in local instance memory in a form
// most suitable for querying.
//
// Thus policies have 3 representations:
//  * Text protos: that's how they are stored in LUCI Config.
//  * Binary protos: that's how Token Server stores them in the datastore.
//  * Queryable state: that's how Token Server keeps them in the local memory.
package policy
