// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate cproto -import-path=identity

// Package identity contains the an API to retrieve callers identity.
//
// It exercises exact same authentication paths as regular services. Useful for
// debugging access tokens.
package identity
