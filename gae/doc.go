// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package gae provides a fakable wrapped interface for the appengine SDK's
// APIs. This means that it's possible to mock all of the supported appengine
// APIs for testing (or potentially implement a different backend for them).
//
// gae currently provides interfaces for:
//   * Datastore
//   * Memcache
//   * TaskQueue
//   * GlobalInfo (e.g. Namespace, AppID, etc.)
//
// A package which implements the gae is expected to provide the following:
//   * A package function `Enable(c context.Context, ...) context.Context`
//     This function is expected to add any information to c which is necessary
//     for the rest of its implementations to work. This may be something like
//     an `appengine.Context` or some connection information for an external
//     server. The `...` in the function signature may be any additional data
//     needed.
//   * Any of the package functions:
//
//       UseDS(context.Context) context.Context
//       UseMC(context.Context) context.Context
//		   UseTQ(context.Context) context.Context
//       UseGI(context.Context) context.Context
//
//	   each of which would call gae.Set<service>Factory with the factory
//	   function for that interface type.
//	 * A `Use(context.Context) context.Context` function which calls all of the
//	   `Use*` package functions implemented by the package.
//   * Partially-implemented interfaces should embed one of the Dummy* structs
//     which will panic with an appropriate error for unimplemented
//     methods.
//
// see "infra/gae/libs/gae/oldsdk" for an appengine-backed implementation.
//
// Datastore struct serialization uses the tag name 'gae' instead of 'datastore',
// and uses gae.DSKey but otherwise behaves identically. You must use the
// new-SDK primitive types (like GeoPoint, ByteString, etc.), even when using
// the old-SDK implementation of the factories. This was done to ensure that all
// code written against gae is go-gettable, and was deemed as less-annoying than
// re-defining all of the types identically.
//
// Kinds/Keys work similarly to how they work in goon (except you no longer use
// the goon + datastore tags and key/control-related tags start with $):
//
//   type FooModel struct {
//     _kind     string       `gae:"$kind,Foo"`
//     Id        string       `gae:"$id"`
//     Parent    DSKey        `gae:"$parent"`
//
//     privateSkipped bool
//     AutoSave       int
//     NoSaveField    string `gae:"-"`
//     NoIndexField   float  `gae:",noindex"`
//     DiffName       string `gae:"coolName"`
//   }
//
//   func (FooModel) GetCachePolicy() DSCacheFlags { return DSCache }
package gae
