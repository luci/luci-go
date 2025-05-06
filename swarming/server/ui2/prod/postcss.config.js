// Copyright 2022 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// postcss config used in all production webpack.common.js configs.
module.exports = {
  plugins: [
    require("autoprefixer")(),
    require("cssnano")({
      // Since cssnano ToT is at >4, the docs on the website are incorrect
      // as to what is and is not on by default. Further, the names
      // appear to have changed slightly. Until pulito is upgraded to
      // use cssnano 4.0, the best place to look for the real names is
      // https://github.com/cssnano/cssnano/blob/v3.10.0/metadata.toml
      "postcss-reduce-idents": false,
      "postcss-discard-overridden": false,
      "postcss-discard-unused": false,
    }),
  ],
};
