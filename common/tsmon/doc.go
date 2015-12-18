// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tsmon contains global state and utility functions for configuring
// and interacting with tsmon.  This has a similar API to the infra-python
// ts_mon module.
//
// Users of tsmon should call InitializeFromFlags from their application's main
// function.
package tsmon
