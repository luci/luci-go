// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package signature

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

func downloadCert(client *http.Client, primaryURL string) (body []byte, err error) {
	resp, err := client.Get(fmt.Sprintf("%s/auth/api/v1/server/certificates", primaryURL))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("during getting certs from %v, got status code %v; want 200", primaryURL, resp.StatusCode)
	}
	body, err = ioutil.ReadAll(resp.Body)
	return body, err
}
