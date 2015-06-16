// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package retry

import (
	"golang.org/x/net/context"
)

// Factory generates a new Iterator instance.
type Factory func(context.Context) Iterator

type retryKeyType int

const retryKey retryKeyType = 0

// Use installs a retry Factory into the current context.
func Use(ctx context.Context, f Factory) context.Context {
	return context.WithValue(ctx, retryKey, f)
}

// GetFactory returns the retry Factory installed in the supplied context, or
// nil if none is installed.
//
// If no Factory is installed, the Default Factory will be used.
func GetFactory(ctx context.Context) Factory {
	if f, ok := ctx.Value(retryKey).(Factory); ok {
		return f
	}
	return Default
}

// Get returns a new retry Iterator instance based on the installed Factory.
func Get(ctx context.Context) Iterator {
	return GetFactory(ctx)(ctx)
}
