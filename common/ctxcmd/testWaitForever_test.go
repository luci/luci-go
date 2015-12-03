// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ctxcmd

import (
	"fmt"
	"os"
	"os/signal"
	"testing"
	"time"
)

func TestWaitForever(t *testing.T) {
	if !isHelperTest() {
		return
	}

	// Set up a signal handler. If we get interrupted, exit with "5".
	signalC := make(chan os.Signal)
	signal.Notify(signalC, os.Interrupt)
	go func() {
		for sig := range signalC {
			fmt.Println("Got signal:", sig)
			os.Exit(5)
		}
	}()

	fmt.Println(waitForeverReady)
	for {
		time.Sleep(time.Second)
		fmt.Println("(Tick)")
	}
}
