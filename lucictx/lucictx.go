// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package lucictx implements a Go client for the protocol defined here:
//   https://github.com/luci/luci-py/blob/master/client/LUCI_CONTEXT.md
//
// It differs from the python client in a couple ways:
//   * The initial LUCI_CONTEXT value is captured once at application start, and
//     the environment variable is REMOVED.
//   * Writes are cached into the golang context.Context, not a global variable.
//   * The LUCI_CONTEXT environment variable is not changed automatically when
//     using the Set function. To pass the new context on to a child process,
//     you must use the Export function to dump the current context state to
//     disk.
package lucictx

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"reflect"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
)

// EnvKey is the environment variable key for the LUCI_CONTEXT file.
const EnvKey = "LUCI_CONTEXT"

// lctx is wrapper around top-level JSON dict of a LUCI_CONTEXT file.
//
// Note that we must use '*json.RawMessage' as dict value type since only
// pointer *json.RawMessage type implements json.Marshaler and json.Unmarshaler
// interfaces. Without '*' the JSON library treats json.RawMessage as []byte and
// marshals it as base64 blob.
type lctx map[string]*json.RawMessage

func (l lctx) clone() lctx {
	ret := make(lctx, len(l))
	for k, v := range l {
		ret[k] = v
	}
	return ret
}

var lctxKey = "Holds the current lctx"

// This is the LUCI_CONTEXT loaded from the environment once when the process
// starts.
var externalContext = extractFromEnv(os.Stderr)

func extractFromEnv(out io.Writer) lctx {
	path := os.Getenv(EnvKey)
	// We unset this here so that child processes don't accidentally inherit
	// a stale LUCI_CONTEXT environment variable.
	os.Unsetenv(EnvKey)
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(out, "Could not open LUCI_CONTEXT file %q: %s", path, err)
		return nil
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	dec.UseNumber()
	tmp := map[string]interface{}{}
	if err := dec.Decode(&tmp); err != nil {
		fmt.Fprintf(out, "Could not decode LUCI_CONTEXT file %q: %s", path, err)
		return nil
	}

	ret := make(lctx, len(tmp))
	for k, v := range tmp {
		if reflect.TypeOf(v).Kind() != reflect.Map {
			fmt.Fprintf(out, "Could not reencode LUCI_CONTEXT file %q, section %q: Not a map.", path, k)
			continue
		}
		item, _ := json.Marshal(v)
		// This section just came from json.Unmarshal, so we know that json.Marshal
		// will work on it.
		raw := json.RawMessage(item)
		ret[k] = &raw
	}
	return ret
}

func getCurrent(ctx context.Context) lctx {
	if l := ctx.Value(&lctxKey); l != nil {
		return l.(lctx)
	}
	return externalContext
}

// Get retrieves the current section from the current LUCI_CONTEXT, and
// deserializes it into out. Out may be any target for json.Unmarshal. If the
// section exists, it deserializes it into the provided out object. If not, then
// out is unmodified.
func Get(ctx context.Context, section string, out interface{}) error {
	data := getCurrent(ctx)[section]
	if data == nil || len(*data) == 0 {
		return nil
	}
	return json.Unmarshal(*data, out)
}

// Lookup retrieves the current section from the current LUCI_CONTEXT, and
// deserializes it into out. Out may be any target for json.Unmarshal. It
// returns a deserialization error (if any), and a boolean indicating if the
// section was actually found.
func Lookup(ctx context.Context, section string, out interface{}) (bool, error) {
	data, ok := getCurrent(ctx)[section]
	if data == nil || len(*data) == 0 {
		return ok, nil
	}
	return ok, json.Unmarshal(*data, out)
}

// Set writes the json serialization of `in` as the given section into the
// LUCI_CONTEXT, returning the new ctx object containing it. This ctx can be
// passed to Export to serialize it to disk.
//
// If in is nil, it will clear that section of the LUCI_CONTEXT.
//
// The returned context is always safe to use, even if this returns an error.
// This only returns an error if `in` cannot be marshalled to a JSON Object.
func Set(ctx context.Context, section string, in interface{}) (context.Context, error) {
	err := error(nil)
	data := json.RawMessage(nil)
	if in != nil {
		if data, err = json.Marshal(in); err != nil {
			return ctx, err
		}
		if data[0] != '{' {
			return ctx, errors.New("LUCI_CONTEXT sections must always be JSON Objects")
		}
	}
	cur := getCurrent(ctx)
	if _, alreadyHas := cur[section]; data == nil && !alreadyHas {
		// Removing a section which is already missing is a no-op
		return ctx, nil
	}
	newLctx := cur.clone()
	if data == nil {
		delete(newLctx, section)
	} else {
		newLctx[section] = &data
	}
	return context.WithValue(ctx, &lctxKey, newLctx), nil
}

// Export takes the current LUCI_CONTEXT information from ctx, writes it to
// a file and returns a wrapping Exported object. This exported value must then
// be installed into the environment of any subcommands (see the methods on
// Exported).
//
// It is required that the caller of this function invoke Close() on the
// returned Exported object, or they will leak temporary files.
func Export(ctx context.Context, dir string) (Exported, error) {
	cur := getCurrent(ctx)
	if len(cur) == 0 {
		return &nullExport{}, nil
	}

	if dir == "" {
		dir = os.TempDir()
	}
	f, err := ioutil.TempFile(dir, "luci_context.")
	if err != nil {
		return nil, errors.Annotate(err).Reason("creating luci_context file").Err()
	}

	l := &liveExport{path: f.Name()}
	data, _ := json.Marshal(cur)
	_, err = f.Write(data)
	f.Close() // intentionally do this even on error.
	if err != nil {
		l.Close() // cleans up the tempfile
		return nil, errors.Annotate(err).Reason("writing luci_context").Err()
	}
	return l, nil
}
