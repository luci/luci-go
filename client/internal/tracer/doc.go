// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package tracer implements code to generate Chrome-compatible traces.
//
// Since there is no thread id concept in Go, pseudo process id and pseudo
// thread id are used. These are defined at application level relative to the
// application-specific context.
//
// See https://github.com/google/trace-viewer for more information.
package tracer
