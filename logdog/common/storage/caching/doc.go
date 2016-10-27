// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package caching implements a Storage wrapper that implements caching on
// reads, and is backed by another Storage instance. The caching layer is
// intentionally simple.
package caching
