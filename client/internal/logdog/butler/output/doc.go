// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package output contains interfaces and implementations for Butler Outputs,
// which are responsible for delivering Butler protobufs to LogDog collection
// endpoints.
//
// Output instance implementations must be goroutine-safe. The Butler may elect
// to output multiple messages at the same time.
//
// The package current provides the following implementations:
//   - pubsub: Write logs to Google Cloud Pub/Sub.
//   - log: (Debug/testing) data is dumped to the installed Logger instance.
package output
