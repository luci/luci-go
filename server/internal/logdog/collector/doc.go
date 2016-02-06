// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package collector implements the LogDog Collector daemon's log parsing and
// registration logic.
//
// The LogDog Collector is responsible for ingesting logs from the Butler sent
// via transport, stashing them in intermediate storage, and registering them
// with the LogDog Coordinator service.
package collector
