// Copyright 2023 The LUCI Authors.
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

import { useEffect } from 'react';
import { useParams } from 'react-router-dom';

import { REFRESH_AUTH_CHANNEL } from '../libs/constants';

export function AuthChannelClosePage() {
  const { channelId } = useParams();

  useEffect(() => {
    new BroadcastChannel(channelId!).postMessage('close');
    REFRESH_AUTH_CHANNEL.postMessage('refresh');
  });

  return <div css={{ margin: '20px' }}>You can close this page if it's not automatically closed</div>;
}
