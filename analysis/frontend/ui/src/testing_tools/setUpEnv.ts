// Copyright 2022 The LUCI Authors.
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
/* eslint-disable @typescript-eslint/ban-ts-comment */

import {
  TextDecoder,
  TextEncoder,
} from 'util';
import { extend as dayjsextend } from 'dayjs';
import localizedFormat from 'dayjs/plugin/localizedFormat';
import relativeTime from 'dayjs/plugin/relativeTime';
import UTC from 'dayjs/plugin/utc';
import fetch from 'node-fetch';

/**
 * jsdom doesn't have those by default, we need to add them for fetch testing.
 */
global.TextEncoder = TextEncoder;

// @ts-ignore
global.TextDecoder = TextDecoder;

// @ts-ignore
global.fetch = fetch;

window.luciAnalysisHostname = self.location.host;
window.monorailHostname = 'crbug.com';

dayjsextend(relativeTime);
dayjsextend(UTC);
dayjsextend(localizedFormat);
