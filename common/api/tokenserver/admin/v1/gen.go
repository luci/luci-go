// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

//go:generate cproto
//go:generate svcdec -type CertificateAuthoritiesServer
//go:generate svcdec -type ServiceAccountsServer

// Package admin contains The Token Server Administrative API.
//
// Services defines here are used by service administrators.
package admin
