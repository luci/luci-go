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

package analytics

// universal.go has logic for generating Google Universal Analytics snippets.

import (
	"fmt"
	"html/template"
	"regexp"
)

var rAllowed = regexp.MustCompile(`UA-\d+-\d+`)

// TODO(crbug/1258632): temporary hack. This should be made private and the
// module should provide public method to read cached analytic.js from
// context like `GTagSnippet`.
func MakeUASnippet(trackingID string) template.HTML {
	return template.HTML(fmt.Sprintf(`
<script>
setTimeout(function() {
	(function(i,s,o,g,r,a,m){i['GoogleAnalyticsObject']=r;i[r]=i[r]||function(){
	(i[r].q=i[r].q||[]).push(arguments)},i[r].l=1*new Date();a=s.createElement(o),
	m=s.getElementsByTagName(o)[0];a.async=1;a.src=g;m.parentNode.insertBefore(a,m)
	})(window,document,'script','https://www.google-analytics.com/analytics.js','ga');

	ga('create', '%s', 'auto');
	ga('send', 'pageview');
}, 0);
</script>
`, template.JSEscapeString(trackingID)))
}
