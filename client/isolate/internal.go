// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolate

import (
	"log"

	"github.com/maruel/ut"
)

// hacky stuff for faster debugging.
func assertNoError(err error) {
	if err == nil {
		return
	}
	log.Panicf(ut.Decorate("assertion failed due to error %s."), err)
}

func assert(condition bool, info ...interface{}) {
	if condition {
		return
	}
	if len(info) == 0 {
		log.Panic(ut.Decorate("assertion failed"))
	} else if format, ok := info[0].(string); ok {
		log.Panicf(ut.Decorate("assertion failed: "+format), info[1:]...)
	}
}
