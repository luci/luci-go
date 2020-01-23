const fs = require('fs');
const glob = require('glob');
const util = require('util');

const globPromise = util.promisify(glob)

module.exports = class TypedConstructableStyleSheetPlugin {
    constructor({globPattern}) {
        this.globPattern = globPattern;
    }

    apply(compiler) {
        compiler.hooks.run.tapPromise('TypedConstructableStyleSheetPlugin', async () => {
            const files = await globPromise(this.globPattern);
            files.forEach((file) => fs.writeFileSync(`${file}.d.ts`, 'export = CSSStyleSheet;'));
        });

        let first = true;
        compiler.hooks.watchRun.tapPromise('TypedConstructableStyleSheetPlugin', async () => {
            if (!first) {
                return;
            }
            first = false;
            const files = await globPromise(this.globPattern);
            files.forEach((file) => fs.writeFileSync(`${file}.d.ts`, 'export = CSSStyleSheet;'));
        });
    }
}
