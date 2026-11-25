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

/** @type {import('eslint').ESLint.Plugin} */
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
        if (/(?:^|[\\/])src[\\/]fleet[\\/]features\.ts$/.test(filename)) {
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
            if (/(?:_old|_deprecated)/i.test(node.source.value)) {
              context.report({ node, messageId: 'noLegacy' });
            }
          },
        };
      },
    },
    'no-hardcoded-colors': {
      meta: {
        type: 'problem',
        messages: {
          rawColor:
            'Do not hardcode color literal "{{value}}" outside src/fleet/theme/. ' +
            'Use tokens from @/fleet/theme/tokens or a recipe from @/fleet/theme/recipes.',
          muiGrey:
            'Do not use static MUI palette color ("{{value}}") outside src/fleet/theme/; ' +
            'use fleetColors or toneColors from @/fleet/theme/tokens.',
          forbiddenImport:
            'Do not import "{{source}}" outside src/fleet/theme/. ' +
            'Consume theme-aware tokens from @/fleet/theme/tokens instead.',
          tokenColorMath:
            'Do not pass CSS var() token "{{token}}" to {{fn}}(); ' +
            'read concrete colors from theme.palette or add a token in src/fleet/theme/palettes.ts.',
        },
      },
      create(context) {
        const HEX_IN_VALUE_RE =
          /(?:^|[\s:(,])(#[0-9a-f]{3}|#[0-9a-f]{4}|#[0-9a-f]{6}|#[0-9a-f]{8})(?:$|[\s;),!])/i;
        const PROSE_REF_RE =
          /\b(?:cl|bug|issue|pr|rank|item|step|row|col|id|ticket|no\.?)\s+#[0-9a-f]+/i;
        const COLOR_FN_RE = /\b(?:rgba?|hsla?)\(\s*\d/i;
        const MUI_STATIC_COLOR_STR_RE =
          /^(?:grey\.(?:\d+|A\d+)|common\.(?:white|black))$/;
        const TOKEN_COLOR_MATH_FNS = new Set([
          'alpha',
          'darken',
          'lighten',
          'emphasize',
          'getContrastText',
        ]);
        const FLEET_TOKEN_OBJECTS = new Set([
          'fleetColors',
          'toneColors',
          'statusTones',
          'healthStateColors',
        ]);
        const COLOR_PROP_NAMES = new Set([
          'color',
          'backgroundColor',
          'background',
          'bgcolor',
          'borderColor',
          'borderTopColor',
          'borderBottomColor',
          'borderLeftColor',
          'borderRightColor',
          'outlineColor',
          'fill',
          'stroke',
          'textDecorationColor',
          'caretColor',
          'stopColor',
        ]);
        const NAMED_COLORS = new Set([
          'white',
          'black',
          'red',
          'green',
          'blue',
          'gray',
          'grey',
          'orange',
          'yellow',
          'purple',
          'pink',
          'cyan',
          'magenta',
          'brown',
          'silver',
          'gold',
          'navy',
          'teal',
          'maroon',
          'lime',
          'olive',
          'aqua',
          'fuchsia',
          'coral',
          'crimson',
          'indigo',
          'violet',
          'tomato',
          'salmon',
          'beige',
          'ivory',
          'khaki',
          'plum',
          'tan',
        ]);

        const getPropName = (node) => {
          if (!node) return null;
          if (node.type === 'Property') {
            if (node.key.type === 'Identifier' && !node.computed) {
              return node.key.name;
            }
            if (
              node.key.type === 'Literal' &&
              typeof node.key.value === 'string'
            ) {
              return node.key.value;
            }
          }
          if (
            node.type === 'JSXAttribute' &&
            node.name &&
            node.name.type === 'JSXIdentifier'
          ) {
            return node.name.name;
          }
          return null;
        };

        const getRootIdentifier = (expr) => {
          let cur = expr;
          while (cur && cur.type === 'MemberExpression') {
            cur = cur.object;
          }
          return cur && cur.type === 'Identifier' ? cur.name : null;
        };

        const checkString = (node, str) => {
          if (typeof str !== 'string' || !str) return;
          // Ignore URLs (e.g. https://...#fragment) and SVG url(#id) refs.
          if (/^https?:\/\//i.test(str) || /url\(\s*#/i.test(str)) return;

          if (MUI_STATIC_COLOR_STR_RE.test(str)) {
            context.report({
              node,
              messageId: 'muiGrey',
              data: { value: str },
            });
            return;
          }
          const fnMatch = str.match(COLOR_FN_RE);
          if (fnMatch) {
            context.report({
              node,
              messageId: 'rawColor',
              data: { value: fnMatch[0] + '...)' },
            });
            return;
          }
          const hexMatch =
            !PROSE_REF_RE.test(str) && str.match(HEX_IN_VALUE_RE);
          if (hexMatch) {
            context.report({
              node,
              messageId: 'rawColor',
              data: { value: hexMatch[1] },
            });
            return;
          }
          const lower = str.trim().toLowerCase();
          if (NAMED_COLORS.has(lower)) {
            let cur = node;
            let parent = node.parent;
            while (
              parent &&
              (parent.type === 'JSXExpressionContainer' ||
                (parent.type === 'ConditionalExpression' &&
                  (parent.consequent === cur || parent.alternate === cur)) ||
                (parent.type === 'LogicalExpression' &&
                  (parent.right === cur ||
                    (parent.left === cur && parent.operator !== '&&'))))
            ) {
              cur = parent;
              parent = parent.parent;
            }
            const propName = getPropName(parent);
            if (propName && COLOR_PROP_NAMES.has(propName)) {
              context.report({
                node,
                messageId: 'rawColor',
                data: { value: str },
              });
            }
          }
        };

        return {
          ImportDeclaration(node) {
            const src = node.source && node.source.value;
            if (typeof src !== 'string') return;
            if (
              src === '@mui/material/colors' ||
              src.startsWith('@mui/material/colors/') ||
              /(?:^@\/fleet\/theme|\.\.?\/.*theme)\/(?:colors|palettes)$/.test(
                src,
              )
            ) {
              context.report({
                node,
                messageId: 'forbiddenImport',
                data: { source: src },
              });
              return;
            }
            if (src === '@mui/material') {
              node.specifiers.forEach((spec) => {
                if (spec.imported && spec.imported.name === 'colors') {
                  context.report({
                    node: spec,
                    messageId: 'forbiddenImport',
                    data: { source: '@mui/material (colors)' },
                  });
                }
              });
            }
          },
          Literal(node) {
            if (typeof node.value === 'string') {
              checkString(node, node.value);
            }
          },
          TemplateElement(node) {
            if (node.value && typeof node.value.raw === 'string') {
              const firstQuasi =
                node.parent &&
                node.parent.type === 'TemplateLiteral' &&
                node.parent.quasis[0] &&
                node.parent.quasis[0].value.raw;
              if (
                typeof firstQuasi === 'string' &&
                /^https?:\/\//i.test(firstQuasi)
              ) {
                return;
              }
              checkString(node, node.value.raw);
            }
          },
          MemberExpression(node) {
            // Catch theme.palette.grey[400], palette.grey[500], palette.common.white
            if (
              node.property &&
              node.property.type === 'Identifier' &&
              (node.property.name === 'grey' ||
                node.property.name === 'common') &&
              ((node.object &&
                node.object.type === 'Identifier' &&
                node.object.name === 'palette') ||
                (node.object &&
                  node.object.type === 'MemberExpression' &&
                  node.object.property &&
                  node.object.property.type === 'Identifier' &&
                  node.object.property.name === 'palette'))
            ) {
              context.report({
                node,
                messageId: 'muiGrey',
                data: { value: `palette.${node.property.name}` },
              });
            }
          },
          CallExpression(node) {
            const fnName =
              node.callee.type === 'Identifier'
                ? node.callee.name
                : node.callee.type === 'MemberExpression' &&
                    node.callee.property.type === 'Identifier'
                  ? node.callee.property.name
                  : null;
            if (!fnName || !TOKEN_COLOR_MATH_FNS.has(fnName)) return;
            const firstArg = node.arguments && node.arguments[0];
            const rootObj = getRootIdentifier(firstArg);
            if (rootObj && FLEET_TOKEN_OBJECTS.has(rootObj)) {
              context.report({
                node: firstArg,
                messageId: 'tokenColorMath',
                data: { token: rootObj, fn: fnName },
              });
            }
          },
        };
      },
    },
  },
};
