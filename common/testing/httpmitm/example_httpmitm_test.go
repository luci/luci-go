// Copyright 2015 The LUCI Authors.
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

package httpmitm

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
)

func getBody(s string) (body []string) {
	pastHeaders := false
	for _, part := range strings.Split(s, "\r\n") {
		if pastHeaders {
			body = append(body, part)
		} else if part == "" {
			pastHeaders = true
		}
	}
	return
}

func Example() {
	// Setup test server.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, Client!"))
	}))
	defer ts.Close()

	// Setup instrumented client.
	type record struct {
		o Origin
		d string
	}
	var records []*record
	client := http.Client{
		Transport: &Transport{
			Callback: func(o Origin, data []byte, err error) {
				records = append(records, &record{o, string(data)})
			},
		},
	}

	_, err := client.Post(ts.URL, "test", bytes.NewBufferString("Hail, Server!"))
	if err != nil {
		return
	}

	// There should be two records: request and response.
	for idx, r := range records {
		fmt.Printf("%d) %s\n", idx, r.o)
		for _, line := range getBody(r.d) {
			fmt.Println(line)
		}
	}

	// Output:
	// 0) Request
	// Hail, Server!
	// 1) Response
	// Hello, Client!
}
