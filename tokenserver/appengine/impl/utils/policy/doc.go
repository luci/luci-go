// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
