#!/usr/bin/env lucicfg

# This example shows a bare minimum needed to declare a builder. This builder is
# not really functional, since it still misses many important parts (e.g. a list
# of Swarming dimensions).

luci.project(
    name = "hello-world",
    buildbucket = "cr-buildbucket.appspot.com",
    swarming = "chromium-swarm.appspot.com",
)

luci.bucket(name = "my-bucket")

luci.builder(
    name = "my-builder",
    bucket = "my-bucket",
    executable = luci.recipe(
        name = "my-recipe",
        cipd_package = "recipe/bundle/package",
    ),
)
