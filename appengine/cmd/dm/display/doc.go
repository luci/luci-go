// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package display includes the 'display' objects for the DM service. These are
// the Cloud Endpoints-compatible and user-visible JSON objects that the DM
// service returns. Most of the code in here is boilerplate code to ensure
// that display objects can be merged together, preserving sorted order.
//
// TODO(iannucci): determine if there's a way to use go generate to deduplicate
// some of this copy/paste code.
package display
