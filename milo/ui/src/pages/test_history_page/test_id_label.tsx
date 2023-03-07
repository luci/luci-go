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

export interface TestIdLabelProps {
  readonly realm: string;
  readonly testId: string;
}

export function TestIdLabel({ realm, testId }: TestIdLabelProps) {
  return (
    <table
      css={{
        width: '100%',
        backgroundColor: 'var(--block-background-color)',
        padding: '6px 16px',
        fontFamily: "'Google Sans', 'Helvetica Neue', sans-serif",
        fontSize: '14px',
      }}
    >
      <tbody>
        <tr>
          <td
            css={{
              color: 'var(--light-text-color)',
              /* Shrink the first column */
              width: '0px',
            }}
          >
            Realm
          </td>
          <td>{realm}</td>
        </tr>
        <tr>
          <td css={{ color: 'var(--light-text-color)' }}>Test</td>
          <td>{testId}</td>
        </tr>
      </tbody>
    </table>
  );
}
