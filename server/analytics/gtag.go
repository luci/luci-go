// Copyright 2021 The LUCI Authors.
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

package analytics

// gtag.go has logic for generating Google Analytics 4 (gtag.js) snippet.

import (
	"fmt"
	"html/template"
	"regexp"
)

var rGA4Allowed = regexp.MustCompile(`^G-[A-Z0-9]+$`)

// makeGTagSnippet returns an HTML snippet for Google Analytics 4.
func makeGTagSnippet(measurementID string) template.HTML {
	return template.HTML(
		fmt.Sprintf(`
			<!-- Global site tag (gtag.js) - Google Analytics -->
			<script async src="https://www.googletagmanager.com/gtag/js?id=%s"></script>
			<script>
			window.dataLayer = window.dataLayer || [];
			function gtag(){dataLayer.push(arguments);}
			gtag('js', new Date());

			gtag('config', '%s');
			</script>`,
			measurementID, measurementID,
		),
	)
}
