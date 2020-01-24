const loaderUtils = require('loader-utils');
const fs = require('fs');

module.exports = function() {}

module.exports.pitch = function(remainingRequest) {
    if (this.cacheable) {
        this.cacheable();
    }

    return `
        var result = require(${loaderUtils.stringifyRequest(this, '!!' + remainingRequest)});
        const sheet = new CSSStyleSheet();
        sheet.replace(result);
        module.exports = sheet;
    `;
}
