// Copyright 2017 The LUCI Authors.
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

package handlers

import (
	"fmt"
	"net/http"

	"go.chromium.org/luci/server/router"
)

// openSearchXML is the template used to serve the OpenSearch Description Document.
// This needs to be a template because the URL template must be a fully qualified
// URL with the hostname.
// See http://www.opensearch.org/Specifications/OpenSearch/1.1#OpenSearch_description_document
var openSearchXML = `<?xml version="1.0" encoding="UTF-8"?>
<OpenSearchDescription xmlns="http://a9.com/-/spec/opensearch/1.1/">
  <ShortName>LUCI</ShortName>
  <Description>
    Layered Universal Continuous Integration - A cloud based CI solution.
  </Description>
  <Url type="text/html" template="https://%s/search/?q={searchTerms}" />
</OpenSearchDescription>`

// searchXMLHandler returns the opensearch document for this domain.
func searchXMLHandler(c *router.Context) {
	c.Writer.Header().Set("Content-Type", "application/opensearchdescription+xml")
	c.Writer.WriteHeader(http.StatusOK)
	fmt.Fprintf(c.Writer, openSearchXML, c.Request.Host)
}
