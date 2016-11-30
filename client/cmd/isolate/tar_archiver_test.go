// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"strconv"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/luci/luci-go/common/isolated"
)

func fakeOsOpen(name string) (io.ReadCloser, error) {
	var r io.Reader
	switch {
	case name == "/err/open":
		return nil, fmt.Errorf("failed to open %q", name)
	case name == "/err/read":
		r = iotest.TimeoutReader(strings.NewReader("sample-file"))
	case strings.HasPrefix(name, "/size/"):
		size, _ := strconv.Atoi(strings.TrimPrefix(name, "/size/"))
		r = strings.NewReader(strings.Repeat("x", size))
	default:
		r = strings.NewReader(fmt.Sprintf("I am the file with name %q", name))
	}
	return ioutil.NopCloser(r), nil
}

func TestTarArchiver(t *testing.T) {
	oldOpen := osOpen
	osOpen = fakeOsOpen
	defer func() { osOpen = oldOpen }()

	var tars []*Tar
	callback := func(tar *Tar) {
		tars = append(tars, tar)
	}

	ta := NewTarAchiver(callback)

	for len(tars) < 20 {
		// Add tars until we have allocated all possible buffers.
		if err := ta.AddItem(&Item{
			Path: "/size/10000",
			Size: 10000,
		}); err != nil {
			log.Fatalf("AddItem returned error %v", err)
		}
	}

	// Check all the existing tar files: they should all have the same contents.
	// (These are all golden values).
	for i, tar := range tars {
		if got, want := tar.FileCount, 931; got != want {
			t.Fatalf("tar[%d].FileCount = %d, want %d", i, got, want)
		}
		if got, want := tar.FileSize, int64(9310000); got != want {
			t.Fatalf("tar[%d].FileSize = %d, want %d", i, got, want)
		}
		if got, want := tar.Digest, isolated.HexDigest("9c4c0e4bd0103b630875c49360cef44d1638a66e"); got != want {
			t.Fatalf("tar[%d].Digest = %q, want %q", i, got, want)
		}
	}

	// Check that the next call to AddItem will hang until we release a tar.
	addc, releasec := make(chan bool), make(chan bool)
	go func() {
		ta.AddItem(&Item{
			Path: "/size/10000",
			Size: 10000,
		})
		close(addc)
	}()
	go func() {
		tars[0].Release()
		close(releasec)
	}()
	select {
	case <-addc:
		t.Fatal("ta.AddItem did not block on release")
	case <-releasec:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("Timeout waiting for AddItem/Release")
	}
	select {
	case <-addc:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("Timeout waiting for AddItem")
	}

	// Flush out the last item, and check that it contains a single file.
	ta.Flush()
	i, tar := 20, tars[20]
	if got, want := tar.FileCount, 1; got != want {
		t.Fatalf("tar[%d].FileCount = %d, want %d", i, got, want)
	}
	if got, want := tar.FileSize, int64(10000); got != want {
		t.Fatalf("tar[%d].FileSize = %d, want %d", i, got, want)
	}
	if got, want := tar.Digest, isolated.HexDigest("3659412a18bdea98e48c744c498a7fcc7694215d"); got != want {
		t.Fatalf("tar[%d].Digest = %q, want %q", i, got, want)
	}
}

func TestTarArchiver_Error(t *testing.T) {
	oldOpen := osOpen
	osOpen = fakeOsOpen
	defer func() { osOpen = oldOpen }()

	ta := NewTarAchiver(func(*Tar) {})

	if err := ta.AddItem(&Item{
		Path: "/err/open",
		Size: 42,
	}); err == nil {
		t.Errorf("ta.AddItem(%q) got nil error, want non-nil", "/err/open")
	}
	if err := ta.AddItem(&Item{
		Path: "/err/read",
		Size: 42,
	}); err == nil {
		t.Errorf("ta.AddItem(%q) got nil error, want non-nil", "/err/open")
	}
}
