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

// How to authenticate an RPC request.
export enum AuthMethod {
  // Do not send any credentials.
  Anonymous = 'anonymous',
  // Send an OAuth access token.
  OAuth = 'oauth',
}

export interface Props {
  selected: AuthMethod;
  onChange: (method: AuthMethod) => void;
  oauthClientId: string;
  disabled?: boolean,
}

// Allows to select an authentication method to use for sending RPCs.
export const AuthSelector = (props: Props) => {
  return (
    <div>
      <h4>Authentication</h4>
      <label>
        <input
          type='radio'
          name='auth'
          checked={props.selected === AuthMethod.Anonymous}
          disabled={props.disabled}
          onChange={() => props.onChange(AuthMethod.Anonymous)}
        />
        Call anonymously
      </label>
      <br />
      <label>
        <input
          type='radio'
          name='auth'
          checked={props.selected === AuthMethod.OAuth}
          disabled={props.disabled}
          onChange={() => props.onChange(AuthMethod.OAuth)}
        />
        Send OAuth access token (client ID is ${props.oauthClientId})
      </label>
    </div>
  );
};
