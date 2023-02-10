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

import FormControl from '@mui/material/FormControl';
import FormControlLabel from '@mui/material/FormControlLabel';
import Radio from '@mui/material/Radio';
import RadioGroup from '@mui/material/RadioGroup';

// How to authenticate an RPC request.
export enum AuthMethod {
  // Do not send any credentials.
  Anonymous = 'anonymous',
  // Send an OAuth access token.
  OAuth = 'oauth',
}

/* eslint-disable-next-line @typescript-eslint/no-namespace */
export namespace AuthMethod {
  const storeKey = 'auth_selector.AuthMethod';

  // Loads an AuthMethod from the local storage.
  export const load = (): AuthMethod => {
    const item = localStorage.getItem(storeKey);
    if (item && Object.values(AuthMethod).includes(item as AuthMethod)) {
      return item as AuthMethod;
    }
    return AuthMethod.OAuth; // default
  };

  // Stores AuthMethod into the local storage.
  export const store = (val: AuthMethod) => {
    localStorage.setItem(storeKey, val as string);
  };
}


export interface Props {
  selected: AuthMethod;
  onChange: (method: AuthMethod) => void;
  oauthClientId: string;
  disabled?: boolean,
}

// Allows to select an authentication method to use for sending RPCs.
export const AuthSelector = (props: Props) => {
  const handleChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    props.onChange((event.target as HTMLInputElement).value as AuthMethod);
  };

  return (
    <FormControl disabled={props.disabled}>
      <RadioGroup row value={props.selected} onChange={handleChange}>
        <FormControlLabel
          value={AuthMethod.Anonymous}
          control={<Radio />}
          label='Call anonymously'
        />
        <FormControlLabel
          value={AuthMethod.OAuth}
          control={<Radio />}
          label='Use OAuth access tokens'
        />
      </RadioGroup>
    </FormControl>
  );
};
