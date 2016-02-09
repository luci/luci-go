// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package epfrontend

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	. "github.com/smartystreets/goconvey/convey"
)

type discoveryTestCase struct {
	backend  endpoints.APIDescriptor
	frontend restDescription
}

func loadJSONTestCase(d interface{}, suite, name, kind string) {
	path := filepath.Join(fmt.Sprintf("%s_testdata", suite), fmt.Sprintf("%s_%s.json", name, kind))
	data, err := ioutil.ReadFile(path)
	if err != nil {
		panic(fmt.Errorf("failed to load test data [%s]: %v", path, err))
	}

	if err := json.Unmarshal(data, d); err != nil {
		panic(fmt.Errorf("failed to unmarshal [%s]: %v", path, err))
	}
}

func loadDiscoveryTestCase(name string) *discoveryTestCase {
	tc := discoveryTestCase{}
	loadJSONTestCase(&tc.backend, "discovery", name, "backend")
	loadJSONTestCase(&tc.frontend, "discovery", name, "frontend")
	return &tc
}

func TestBuildRestDescription(t *testing.T) {
	Convey(`A testing APIDescriptor`, t, func() {
		u, err := url.Parse("https://example.com/testing/v1")
		So(err, ShouldBeNil)

		for _, tcName := range []string{
			"basic",
			"query_get",
		} {
			Convey(fmt.Sprintf(`Correctly loads/generates the %q test case.`, tcName), func() {

				tc := loadDiscoveryTestCase(tcName)
				rd, err := buildRestDescription(u, &tc.backend)
				So(err, ShouldBeNil)
				So(rd, ShouldResemble, &tc.frontend)
			})
		}
	})
}
