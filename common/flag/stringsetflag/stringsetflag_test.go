// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package stringsetflag

import (
	"flag"
	"fmt"
	"os"
)

// Example demonstrates how to use stringlistflag.
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
	// -color value
	//     	favorite color, may be repeated.
	// Value is: Red,Violet
	// Likes Blue: false
	// Likes Red: true
}
