// Copyright 2022 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.


import {
  TextDecoder,
  TextEncoder,
} from 'util';

import fetch from 'node-fetch';

/**
 * jsdom doesn't have those by default, we need to add them for fetch testing.
 */
global.TextEncoder = TextEncoder;

// We need to allow the below comments in eslint because Typescript doesn't
// understand that there is a missing decoder in Jest's window
// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
global.TextDecoder = TextDecoder;

// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
global.fetch = fetch;
