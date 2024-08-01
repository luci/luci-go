// Copyright 2024 The LUCI Authors.
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

export * from './params_pager';

export {
  emptyPageTokenUpdater,
  getPageSize,
  getPageToken,
  pageSizeUpdater,
  usePagerContext,

  // Do not export the following. The state should be managed by the
  // `<ParamsPager />`. Otherwise we can't keep track of the previous page
  // tokens.
  //
  // getState,
  // pageTokenUpdater,
} from './context';
