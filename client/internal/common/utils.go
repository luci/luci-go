// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"errors"
	"net/url"
	"os"
	"runtime"
	"strings"
)

// URLToHTTPS ensures the url is https://.
func URLToHTTPS(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil {
		return "", err
	}
	if u.Scheme != "" && u.Scheme != "https" {
		return "", errors.New("Only https:// scheme is accepted. It can be omitted.")
	}
	if !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	if _, err = url.Parse(s); err != nil {
		return "", err
	}
	return s, nil
}

// IsDirectory returns true if path is a directory and is accessible.
func IsDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	return err == nil && fileInfo.IsDir()
}

func IsWindows() bool {
	return runtime.GOOS == "windows"
}
