// Copyright 2026 The LUCI Authors.
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

const fs = require('fs');
const path = require('path');

module.exports = async function (context) {
  const vars = context.vars || {};
  const templatePath = vars._templatePath;
  if (!templatePath) {
    throw new Error('_templatePath must be provided in vars');
  }

  const fullPath = path.resolve(__dirname, templatePath);
  let content = fs.readFileSync(fullPath, 'utf8');

  // Simple substitution for Go template variables {{.var}}
  // We iterate over keys in vars and replace {{.key}} with value
  for (const [key, value] of Object.entries(vars)) {
    if (key === '_templatePath') continue;
    // Replace all occurrences of {{.key}}
    // We escape key just in case, though usually simple strings
    const placeholder = `{{.${key}}}`;
    content = content.split(placeholder).join(value);
  }

  return content;
};
