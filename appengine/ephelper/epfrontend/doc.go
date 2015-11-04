// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package epfrontend implements a Google Cloud Endpoints frontend service.
// The frontend service is toughly coupled to one or more go-endpints services,
// and will forward frontend calls (/_ah/api/...) to the go-endpoints service's
// backend endpoint (/_ah/spi/...).
//
// Because the frontend service is hosted in the same instance as the backend,
// the forwarding will be done locally, and will not require additional network
// operations.
package epfrontend
