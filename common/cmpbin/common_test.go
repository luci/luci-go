// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cmpbin

import (
	"errors"
	"flag"
	"log"
	"time"
)

var seed = flag.Int64("cmpbin.seed", time.Now().UnixNano(), "random seed for testing")

var randomTestSize = 1000

func init() {
	flag.Parse()
	log.Println("cmpbin.seed =", *seed)
}

type fakeWriter struct{ count int }

func (f *fakeWriter) WriteByte(byte) error {
	if f.count == 0 {
		return errors.New("nope")
	}
	f.count--
	return nil
}
