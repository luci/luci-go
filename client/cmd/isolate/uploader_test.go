// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"reflect"
	"sync"
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/isolatedclient"
)

type fakePushService struct {
	isolateService
	mu sync.Mutex

	// sources keeps track of the provided sources, keyed by PushStates.
	sources map[*isolatedclient.PushState]isolatedclient.Source

	// errc is a channel from which errors are pulled, if non-nil.
	errc <-chan error
}

func (f *fakePushService) Push(_ context.Context, ps *isolatedclient.PushState, src isolatedclient.Source) error {
	f.mu.Lock()
	if f.sources == nil {
		f.sources = make(map[*isolatedclient.PushState]isolatedclient.Source)
	}
	f.sources[ps] = src
	f.mu.Unlock()
	if f.errc != nil {
		return <-f.errc
	}
	return nil
}

func (f *fakePushService) Sources() map[*isolatedclient.PushState]isolatedclient.Source {
	f.mu.Lock()
	defer f.mu.Unlock()
	srcs := make(map[*isolatedclient.PushState]isolatedclient.Source, len(f.sources))
	for k, v := range f.sources {
		srcs[k] = v
	}
	return srcs
}

func TestUploader_UploadBytes(t *testing.T) {
	errc := make(chan error)
	svc := &fakePushService{errc: errc}
	uploader := newUploader(context.Background(), svc, 5)

	// Keep track of the mapping of push objects to expected bodies.
	want := make(map[*isolatedclient.PushState][]byte)

	// Send 15 calls to UploadBytes: we expect only the first 10 to be
	// sent through before stuff blocks.
	for i := 0; i < 15; i++ {
		ps, body := &isolatedclient.PushState{}, []byte(fmt.Sprint(i))
		want[ps] = body
		uploader.UploadBytes(fmt.Sprintf("name-%d", i), body, ps, nil)
	}

	// See that at most 10 items arrived in the service so far.
	if got := len(svc.Sources()); got > 5 {
		t.Errorf("Sources uploaded: got %d; want at most 5", got)
	}

	// Check that close does not return until we've unblocked all the pending
	// sends.
	closedc := make(chan bool)
	go func() {
		if err := uploader.Close(); err != nil {
			t.Fatalf("Close got err %v; want nil", err)
		}
		close(closedc)
	}()
	for range want {
		select {
		case errc <- nil:
		case <-closedc:
			t.Fatal("Close returned before Push calls completed")
		}
	}
	<-closedc

	got := make(map[*isolatedclient.PushState][]byte, len(want))
	for ps, src := range svc.Sources() {
		r, err := src()
		if err != nil {
			t.Fatalf("Source for ps %v returned error %v; want nil", ps, err)
		}
		b, err := ioutil.ReadAll(r)
		if err != nil {
			t.Fatalf("Reader for ps %v returned error %v; want nil", ps, err)
		}
		got[ps] = b
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Uploaded sources:\ngot:  %v\nwant: %v", got, want)
	}
}

func TestUpload_UploadFile(t *testing.T) {
	// See tar_archiver_test for the implementation of fakeOsOpen.
	oldOpen := osOpen
	osOpen = fakeOsOpen
	defer func() { osOpen = oldOpen }()

	svc := &fakePushService{}
	uploader := newUploader(context.Background(), svc, 1)

	// Upload 2 files: the second returns an error on open.
	ps1 := &isolatedclient.PushState{}
	uploader.UploadFile(&Item{
		RelPath: "10",
		Path:    "/size/10",
	}, ps1, nil)

	ps2 := &isolatedclient.PushState{}
	uploader.UploadFile(&Item{
		RelPath: "open",
		Path:    "/err/open",
	}, ps2, nil)

	if err := uploader.Close(); err != nil {
		t.Fatalf("Close got err %v; want nil", err)
	}
	sources := svc.Sources()
	if len(sources) != 2 {
		t.Errorf("Got %d sources, want 2\nSources %v", len(sources), sources)
	}

	// The first file has a valid source whose contents is size 10.
	src, ok := sources[ps1]
	if !ok {
		t.Fatalf("Wanted source for ps %v; none found", ps1)
	}
	r, err := src()
	if err != nil {
		t.Fatalf("Source for %v returned error %v; want nil", ps1, err)
	}
	defer r.Close()
	b, err := ioutil.ReadAll(r)
	if err != nil {
		t.Fatalf("Reader for ps %v returned error %v; want nil", ps1, err)
	}
	if got, want := len(b), 10; got != want {
		t.Errorf("Content for ps %v: got %q (len %d), want len %d", ps1, b, got, want)
	}

	// The second file has a source which returns an error.
	src, ok = sources[ps2]
	if !ok {
		t.Fatalf("Wanted source for ps %v; none found", ps2)
	}
	r, err = src()
	if err == nil {
		r.Close()
		log.Fatalf("Source for %v gave nil-err, want non-nil", ps2)
	}
}

func TestUploader_Errors(t *testing.T) {
	errc := make(chan error)
	svc := &fakePushService{errc: errc}
	uploader := newUploader(context.Background(), svc, 1)

	// Make fake uploader fail on the second request.
	go func() {
		errc <- nil
		errc <- errors.New("push failed")
		close(errc)
	}()

	// Send 5 calls to UploadBytes, sending the last 3 only once the first two
	// complete.
	donec := make(chan bool)
	uploader.UploadBytes("name-0", []byte("0"), &isolatedclient.PushState{}, func() {
		donec <- true
	})
	uploader.UploadBytes("name-1", []byte("1"), &isolatedclient.PushState{}, func() {
		donec <- true
	})
	_, _ = <-donec, <-donec // Wait for the first two
	uploader.UploadBytes("name-2", []byte("2"), &isolatedclient.PushState{}, nil)
	uploader.UploadBytes("name-3", []byte("3"), &isolatedclient.PushState{}, nil)
	uploader.UploadBytes("name-4", []byte("4"), &isolatedclient.PushState{}, nil)

	// Close will return the error.
	if err := uploader.Close(); err == nil {
		t.Fatal("Close got nil err; want non-nil", err)
	}

	// Only two of the sources should have made it through to the uploader.
	sources := svc.Sources()
	if len(sources) != 2 {
		t.Errorf("Services had %d sources, want %d\nSources: %v", len(sources), 2, sources)
	}
}
