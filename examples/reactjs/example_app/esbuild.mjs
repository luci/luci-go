// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.


import  { sassPlugin } from 'esbuild-sass-plugin';
import esbuild from 'esbuild';

esbuild.build({
    entryPoints: ['index.tsx'],
    bundle: true,
    outfile: 'dist/main.js',
    minify: true,
    sourcemap: true,
    plugins: [sassPlugin()],
    loader: {
        '.png': 'dataurl',
        '.woff': 'dataurl',
        '.woff2': 'dataurl',
        '.eot': 'dataurl',
        '.ttf': 'dataurl',
        '.svg': 'dataurl',
    },
// eslint-disable-next-line @typescript-eslint/no-unused-vars
}).catch((_) => {
    // eslint-disable-next-line no-undef
    process.exit(1);
});
