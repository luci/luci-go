// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tokenclient implements pRPC client for The Token Server.
//
// It is a thin wrapper around regular pRPC client, that knows how to use
// private keys and certificates to properly prepare requests for the token
// server.
package tokenclient
