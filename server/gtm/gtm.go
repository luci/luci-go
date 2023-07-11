// Copyright 2023 The LUCI Authors.
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

package gtm

// gtm.go has logic for generating Google Tag Manager (gtm.js) snippet.

import (
	"fmt"
	"html/template"
	"regexp"
)

var rGTMAllowed = regexp.MustCompile(`^GTM-[A-Z0-9]+$`)

// makeGTMJSSnippet returns an HTML JS snippet for Google Tag Manager.
func makeGTMJSSnippet(containerID string) template.HTML {
	return template.HTML(
		fmt.Sprintf(`
			<!-- Google Tag Manager -->
			<script>(function(w,d,s,l,i){w[l]=w[l]||[];w[l].push({'gtm.start':
			new Date().getTime(),event:'gtm.js'});var f=d.getElementsByTagName(s)[0],
			j=d.createElement(s),dl=l!='dataLayer'?'&l='+l:'';j.async=true;j.src=
			'https://www.googletagmanager.com/gtm.js?id='+i+dl;f.parentNode.insertBefore(j,f);
			})(window,document,'script','dataLayer','%s');</script>
			<!-- End Google Tag Manager -->`,
			containerID,
		),
	)
}

// makeGTMNoScriptSnippet returns an HTML snippet for Google Tag Manager that
// works in a no-script context.
func makeGTMNoScriptSnippet(containerID string) template.HTML {
	return template.HTML(
		fmt.Sprintf(`
			<!-- Google Tag Manager (noscript) -->
			<noscript><iframe src="https://www.googletagmanager.com/ns.html?id=%s"
			height="0" width="0" style="display:none;visibility:hidden"></iframe></noscript>
			<!-- End Google Tag Manager (noscript) -->`,
			containerID,
		),
	)
}
