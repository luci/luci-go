// Copyright 2018 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package config contains a client to access LUCI configuration service.
//
// WARNING: Large chunks of this package tree are deprecated and will be removed
// at some point. In particular 'appengine/*' and 'server/cfgclient/*' will not
// be made compatible with GAE v2 and will be deleted.
//
// The existing API (exposed via server/cfgclient package) is "pull-based":
// users "pull" configs via cfgclient.Get(...) calls whenever they want.
// In particular they may do it from the data plane (which generally handles
// a lot of QPS). This requires cfgclient.Get(...) to be very fast and very
// reliable. "Very fast and very reliable" doesn't apply to LUCI Config service
// itself at all (nor should it). As a result cfgclient.Get(...) implementation
// had to incorporate some pretty complicated multi-layered cache (that no one
// fully understands anymore).
//
// But turns out the majority of users (but not all) use cfgclient.Get(...) from
// their control plane, i.e. they run some cron job that periodically pulls
// most recent configs, processes them in some way, and stores them somewhere
// where the data plane can reliably reach them. This also gives them
// flexibility to react to diffs in the configs.
//
// For such users the complexity of cfgclient.Get(...) implementation is
// completely unnecessary. It just adds more work to them to configure the
// caching layer.
//
// We are migrating to GAE v2 and GKE, and we no longer assume Cloud Datastore
// and Memcache are ubiquitously present. The existing complicated cfgclient
// code will not work in such environments.
//
// Instead of refactoring it we are planning to remove it completely and change
// the public package API to be "push-based", i.e. we want to *force* users to
// register some callback which receives configs whenever they change. Then
// users may store them whereever they want (like most of them already do in the
// cron jobs).
//
// This approach would result in a vastly simpler config client code and will
// guarantee LUCI Config service is not a dependency of the data plane (so it's
// not a big deal if it goes down).
package config
