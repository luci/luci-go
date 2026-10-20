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

export default {
  rules: {
    'no-window-location-hostname': {
      meta: {
        type: 'problem',
        messages: {
          noHostname:
            'Do not check window.location.hostname directly in Fleet Console; use createFleetFeatureFlag and useFeatureFlag instead.',
        },
      },
      create(context) {
        return {
          MemberExpression(node) {
            if (
              node.object &&
              node.object.type === 'MemberExpression' &&
              node.object.object &&
              node.object.object.name === 'window' &&
              node.object.property &&
              node.object.property.name === 'location' &&
              node.property &&
              node.property.name === 'hostname'
            ) {
              context.report({ node, messageId: 'noHostname' });
            }
          },
        };
      },
    },
    'require-create-fleet-feature-flag': {
      meta: {
        type: 'problem',
        messages: {
          requireFleetFlag:
            'Register Fleet Console flags in src/fleet/features.ts via createFleetFeatureFlag.',
        },
      },
      create(context) {
        const filename =
          context.filename ||
          (context.getFilename ? context.getFilename() : '');
        if (filename.endsWith('features.ts')) {
          return {};
        }
        return {
          ImportDeclaration(node) {
            if (node.source.value === '@/common/feature_flags') {
              node.specifiers.forEach((spec) => {
                if (
                  spec.imported &&
                  spec.imported.name === 'createFeatureFlag'
                ) {
                  context.report({ node: spec, messageId: 'requireFleetFlag' });
                }
              });
            }
          },
        };
      },
    },
    'no-legacy-imports': {
      meta: {
        type: 'problem',
        messages: {
          noLegacy:
            'Do not import legacy *_OLD or *_deprecated modules in new Fleet Console code.',
        },
      },
      create(context) {
        return {
          ImportDeclaration(node) {
            if (/_old|_OLD|_deprecated/.test(node.source.value)) {
              context.report({ node, messageId: 'noLegacy' });
            }
          },
        };
      },
    },
  },
};
