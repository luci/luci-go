// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package backend implements configuration client backend interface and
// associated data types.
//
// The backend package implements a generalized implementation-oriented
// interface to configuration services that can be layered. Various backend
// implementations are best defined out of this package.
//
// All non-user-facing operations of cfgclient will operate on backend types.
package backend
