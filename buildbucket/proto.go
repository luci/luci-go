package buidlbucket

// This file contains helper functions for buildbucketpb package.
// TODO(nodir): move existing helpers from buildbucketpb to this file.

// BuildTokenHeader is the name of gRPC metadata header indicating the build
// token (see BuildSecrets.BuildToken).
// It is required in UpdateBuild RPC.
// Defined in
// https://chromium.googlesource.com/infra/infra/+/c189064/appengine/cr-buildbucket/v2/api.py#35
const BuildTokenHeader = "x-build-token"
