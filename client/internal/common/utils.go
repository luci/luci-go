// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/luci/luci-go/client/internal/imported"
)

var units = []string{"b", "Kib", "Mib", "Gib", "Tib", "Pib", "Eib", "Zib", "Yib"}

// Size represents a size in bytes that knows how to print itself.
type Size int64

func (s Size) String() string {
	v := float64(s)
	i := 0
	for ; i < len(units); i++ {
		if v < 1024. {
			break
		}
		v /= 1024.
	}
	return fmt.Sprintf("%.1f%s", v, units[i])
}

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

// Round rounds a time.Duration at round.
func Round(value time.Duration, resolution time.Duration) time.Duration {
	if value < 0 {
		value -= resolution / 2
	} else {
		value += resolution / 2
	}
	return value / resolution * resolution
}

// IsTerminal returns true if the specified io.Writer is a terminal.
func IsTerminal(out io.Writer) bool {
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	return imported.IsTerminal(int(f.Fd()))
}
