// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gs

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type testReaderClient struct {
	Client

	newReaderErr error
	readErr      error
	closeErr     error

	maxRead      int64
	data         map[Path][]byte
	readers      map[*testReader]struct{}
	totalReaders int
}

func (trc *testReaderClient) NewReader(p Path, offset, length int64) (io.ReadCloser, error) {
	if err := trc.newReaderErr; err != nil {
		return nil, err
	}

	if trc.maxRead > 0 && length > 0 && length > trc.maxRead {
		return nil, fmt.Errorf("maximum read exceeded (%d > %d)", length, trc.maxRead)
	}

	data := trc.data[p]
	if int64(len(data)) < offset {
		return nil, errors.New("reading past EOF")
	}
	data = data[offset:]
	if length >= 0 && length < int64(len(data)) {
		data = data[:length]
	}
	tr := &testReader{
		client: trc,
		data:   data,
	}
	trc.readers[tr] = struct{}{}
	trc.totalReaders++
	return tr, nil
}

type testReader struct {
	client *testReaderClient
	data   []byte
	closed bool
}

func (tr *testReader) checkClosed() error {
	if tr.closed {
		return errors.New("already closed")
	}
	return nil
}

func (tr *testReader) Close() error {
	delete(tr.client.readers, tr)

	if err := tr.client.closeErr; err != nil {
		return err
	}
	if err := tr.checkClosed(); err != nil {
		return err
	}
	tr.closed = true
	return nil
}

func (tr *testReader) Read(b []byte) (amt int, err error) {
	if err = tr.client.readErr; err != nil {
		return
	}

	amt = copy(b, tr.data)
	tr.data = tr.data[amt:]
	if len(tr.data) == 0 {
		err = io.EOF
	}
	return
}

func TestLimitedClient(t *testing.T) {
	t.Parallel()

	Convey(`Testing limited client`, t, func() {
		const path = "gs://bucket/file"
		data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		trc := testReaderClient{
			data: map[Path][]byte{
				path: data,
			},
			readers: map[*testReader]struct{}{},
		}
		defer So(trc.readers, ShouldHaveLength, 0)

		lc := &LimitedClient{
			Client: &trc,
		}

		setLimit := func(limit int) {
			lc.MaxReadBytes = int64(limit)
			trc.maxRead = int64(limit)
		}

		Convey(`With no read limit, can read the full stream.`, func() {
			r, err := lc.NewReader(path, 0, -1)
			So(err, ShouldBeNil)
			defer r.Close()

			d, err := ioutil.ReadAll(r)
			So(err, ShouldBeNil)
			So(d, ShouldResemble, data)
		})

		for _, limit := range []int{-1, 1, 2, 5, len(data) - 1, len(data), len(data) + 1} {
			Convey(fmt.Sprintf(`Variety test: with a read limit of %d`, limit), func() {
				setLimit(limit)

				Convey(`Can read the full stream with no limit.`, func() {
					r, err := lc.NewReader(path, 0, -1)
					So(err, ShouldBeNil)
					defer r.Close()

					d, err := ioutil.ReadAll(r)
					So(err, ShouldBeNil)
					So(d, ShouldResemble, data)
				})

				Convey(`Can read a partial stream.`, func() {
					r, err := lc.NewReader(path, 3, 6)
					So(err, ShouldBeNil)
					defer r.Close()

					d, err := ioutil.ReadAll(r)
					So(err, ShouldBeNil)
					So(d, ShouldResemble, data[3:9])
				})

				Convey(`Can read an offset stream with a limit.`, func() {
					r, err := lc.NewReader(path, 3, 6)
					So(err, ShouldBeNil)
					defer r.Close()

					d, err := ioutil.ReadAll(r)
					So(err, ShouldBeNil)
					So(d, ShouldResemble, data[3:9])
				})
			})
		}

		Convey(`With a read limit of 2`, func() {
			setLimit(2)

			Convey(`Reading full stream creates expected readers.`, func() {
				r, err := lc.NewReader(path, 0, -1)
				So(err, ShouldBeNil)
				defer r.Close()

				d, err := ioutil.ReadAll(r)
				So(err, ShouldBeNil)
				So(d, ShouldResemble, data)
				So(trc.totalReaders, ShouldEqual, 6) // One for each, plus real EOF.
			})

			Convey(`Reading partial stream (even) creates expected readers.`, func() {
				r, err := lc.NewReader(path, 3, 6)
				So(err, ShouldBeNil)
				defer r.Close()

				d, err := ioutil.ReadAll(r)
				So(err, ShouldBeNil)
				So(d, ShouldResemble, data[3:9])
				So(trc.totalReaders, ShouldEqual, 3)
			})

			Convey(`Reading partial stream (odd) creates expected readers.`, func() {
				r, err := lc.NewReader(path, 3, 5)
				So(err, ShouldBeNil)
				defer r.Close()

				d, err := ioutil.ReadAll(r)
				So(err, ShouldBeNil)
				So(d, ShouldResemble, data[3:8])
				So(trc.totalReaders, ShouldEqual, 3)
			})

			Convey(`Configured to error on new reader, returns that error.`, func() {
				trc.newReaderErr = errors.New("test error")
				_, err := lc.NewReader(path, 3, 5)
				So(err, ShouldEqual, trc.newReaderErr)
			})

			Convey(`Configured to error on close, returns that error on read.`, func() {
				r, err := lc.NewReader(path, 0, -1)
				So(err, ShouldBeNil)
				defer r.Close()

				trc.readErr = errors.New("test error")
				_, err = ioutil.ReadAll(r)
				So(err, ShouldEqual, trc.readErr)
			})

			Convey(`Configured to error on read, returns that error.`, func() {
				r, err := lc.NewReader(path, 3, 5)
				So(err, ShouldBeNil)
				defer r.Close()

				trc.closeErr = errors.New("test error")
				_, err = ioutil.ReadAll(r)
				So(err, ShouldEqual, trc.closeErr)
			})
		})
	})
}
