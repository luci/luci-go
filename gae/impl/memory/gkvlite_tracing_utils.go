// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/luci/gkvlite"
)

var logMemCollectionFolder = flag.String(
	"luci.gae.gkvlite_trace_folder", "",
	"Set to a folder path to enable debugging traces to be dumped there. Set to '-' to dump to stdout.")
var logMemCollectionFolderTmp string
var logMemCollectionOnce sync.Once
var logMemCounter uint32
var logMemNameKey = "holds a string indicating the GKVLiteDebuggingTraceName"
var stdoutLock sync.Mutex

func wrapTracingMemStore(store memStore) memStore {
	var writer traceWriter
	logNum := atomic.AddUint32(&logMemCounter, 1) - 1
	collName := fmt.Sprintf("coll%d", logNum)

	if *logMemCollectionFolder == "-" {
		writer = func(format string, a ...interface{}) {
			stdoutLock.Lock()
			defer stdoutLock.Unlock()
			fmt.Printf(format+"\n", a...)
		}
	} else {
		logMemCollectionOnce.Do(func() {
			var err error
			logMemCollectionFolderTmp, err = ioutil.TempDir(*logMemCollectionFolder, "luci-gae-gkvlite_trace")
			if err != nil {
				panic(err)
			}
			logMemCollectionFolderTmp, err = filepath.Abs(logMemCollectionFolderTmp)
			if err != nil {
				panic(err)
			}
			fmt.Fprintf(os.Stderr, "Saving GKVLite trace files to %q\n", logMemCollectionFolderTmp)
		})
		if logMemCollectionFolderTmp == "" {
			panic("unable to create folder for tracefiles")
		}

		lck := sync.Mutex{}
		fname := fmt.Sprintf(filepath.Join(logMemCollectionFolderTmp, fmt.Sprintf("%d.trace", logNum)))
		fil, err := os.Create(fname)
		if err != nil {
			panic(err)
		}
		writer = func(format string, a ...interface{}) {
			lck.Lock()
			defer lck.Unlock()
			fmt.Fprintf(fil, format+"\n", a...)
		}
		runtime.SetFinalizer(&writer, func(_ *traceWriter) { fil.Close() })
	}
	writer("%s := newMemStore()", collName)
	return &tracingMemStoreImpl{store, writer, collName, 0, false}
}

type traceWriter func(format string, a ...interface{})

type tracingMemStoreImpl struct {
	i memStore
	w traceWriter

	collName string
	// for the mutable store, this is a counter that increments for every
	// Snapshot, and for snapshots, this is the number of the snapshot.
	snapNum uint
	isSnap  bool
}

var _ memStore = (*tracingMemStoreImpl)(nil)

func (t *tracingMemStoreImpl) ImATestingSnapshot() {}

func (t *tracingMemStoreImpl) colWriter(action, name string) traceWriter {
	ident := t.ident()
	hexname := hex.EncodeToString([]byte(name))
	writer := func(format string, a ...interface{}) {
		if strings.HasPrefix(format, "//") { // comment
			t.w(format, a...)
		} else {
			t.w(fmt.Sprintf("%s_%s%s", ident, hexname, format), a...)
		}
	}
	writer(" := %s.%s(%q)", ident, action, name)
	return writer
}

func (t *tracingMemStoreImpl) ident() string {
	if t.isSnap {
		return fmt.Sprintf("%s_snap%d", t.collName, t.snapNum)
	}
	return t.collName
}

func (t *tracingMemStoreImpl) GetCollection(name string) memCollection {
	coll := t.i.GetCollection(name)
	if coll == nil {
		t.w("// %s.GetCollection(%q) -> nil", t.ident(), name)
		return nil
	}
	writer := t.colWriter("GetCollection", name)
	return &tracingMemCollectionImpl{coll, writer, 0}
}

func (t *tracingMemStoreImpl) GetCollectionNames() []string {
	t.w("%s.GetCollectionNames()", t.ident())
	return t.i.GetCollectionNames()
}

func (t *tracingMemStoreImpl) GetOrCreateCollection(name string) memCollection {
	writer := t.colWriter("GetOrCreateCollection", name)
	return &tracingMemCollectionImpl{t.i.GetOrCreateCollection(name), writer, 0}
}

func (t *tracingMemStoreImpl) Snapshot() memStore {
	snap := t.i.Snapshot()
	if snap == t.i {
		t.w("// %s.Snapshot() -> self", t.ident())
		return t
	}
	ret := &tracingMemStoreImpl{snap, t.w, t.collName, t.snapNum, true}
	t.w("%s := %s.Snapshot()", ret.ident(), t.ident())
	t.snapNum++
	return ret
}

func (t *tracingMemStoreImpl) IsReadOnly() bool {
	return t.i.IsReadOnly()
}

type tracingMemCollectionImpl struct {
	i           memCollection
	w           traceWriter
	visitNumber uint
}

var _ memCollection = (*tracingMemCollectionImpl)(nil)

func (t *tracingMemCollectionImpl) Name() string {
	return t.i.Name()
}

func (t *tracingMemCollectionImpl) Delete(k []byte) bool {
	t.w(".Delete(%#v)", k)
	return t.i.Delete(k)
}

func (t *tracingMemCollectionImpl) Get(k []byte) []byte {
	t.w(".Get(%#v)", k)
	return t.i.Get(k)
}

func (t *tracingMemCollectionImpl) GetTotals() (numItems, numBytes uint64) {
	t.w(".GetTotals()")
	return t.i.GetTotals()
}

func (t *tracingMemCollectionImpl) MinItem(withValue bool) *gkvlite.Item {
	t.w(".MinItem(%t)", withValue)
	return t.i.MinItem(withValue)
}

func (t *tracingMemCollectionImpl) Set(k, v []byte) {
	t.w(".Set(%#v, %#v)", k, v)
	t.i.Set(k, v)
}

func (t *tracingMemCollectionImpl) VisitItemsAscend(target []byte, withValue bool, visitor gkvlite.ItemVisitor) {
	vnum := t.visitNumber
	t.visitNumber++

	t.w(".VisitItemsAscend(%#v, %t, func(i *gkvlite.Item) bool{ return true }) // BEGIN VisitItemsAscend(%d)", target, withValue, vnum)
	defer t.w("// END VisitItemsAscend(%d)", vnum)
	t.i.VisitItemsAscend(target, withValue, visitor)
}

func (t *tracingMemCollectionImpl) IsReadOnly() bool {
	return t.i.IsReadOnly()
}
