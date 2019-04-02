// Copyright 2019 The LUCI Authors.
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

package text

import (
	"fmt"
)

func ExampleDoc() {
	fmt.Println(Doc(`
		Leading blank line above is removed.

		The indentation on the left is trimmed.

		These 2 lines appear on the same line,
		with a space instead of newline.

		A new paragraph is preserved.
			New line before indented text is preserved.
			Indentation of the intentionally indented text is preserved.

		Trailing blank lines is removed.



	`))

	// Output:
	// Leading blank line above is removed.
	//
	// The indentation on the left is trimmed.
	//
	// These 2 lines appear on the same line, with a space instead of newline.
	//
	// A new paragraph is preserved.
	// 	New line before indented text is preserved.
	// 	Indentation of the intentionally indented text is preserved.
	//
	// Trailing blank lines is removed.
}
