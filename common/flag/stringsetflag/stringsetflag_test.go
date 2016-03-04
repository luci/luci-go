// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package stringsetflag

import (
	"flag"
	"fmt"
	"os"
)

// Example demonstrates how to use flagenum to create bindings for a custom
// type.
func Example() {
	sset := Flag{}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Var(&sset, "color", "favorite color, may be repeated.")
	fs.SetOutput(os.Stdout)

	fs.PrintDefaults()

	// Flag parsing.
	fs.Parse([]string{"-color", "Violet", "-color", "Red", "-color", "Violet"})
	fmt.Printf("Value is: %s\n", sset)

	fmt.Println("Likes Blue:", sset.Data.Has("Blue"))
	fmt.Println("Likes Red:", sset.Data.Has("Red"))

	// Output:
	// -color=: favorite color, may be repeated.
	// Value is: Red,Violet
	// Likes Blue: false
	// Likes Red: true
}
