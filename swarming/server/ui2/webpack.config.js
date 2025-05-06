// Copyright 2022 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// The commonBuilder function generates a common configuration for webpack.
// You can require() it at the start of your webpack.config.js and then make
// modifications to it from there. Users should at a minimum fill in the entry
// points.  See webpack.config.js in this directory as an example.
//
// Usage:
//    A webpack.config.js can be as simple as:
//
//       const commonBuilder = require('pulito');
//       module.exports = (env, argv) => commonBuilder(env, argv, __dirname);
//
//    For an application you need to add the entry points and associate them
//    with HTML files:
//
//        const commonBuilder = require('pulito');
//        const HtmlWebpackPlugin = require('html-webpack-plugin');
//
//        module.exports = (env, argv) => {
//          let common = commonBuilder(env, argv, __dirname);
//          common.entry.index = './pages/index.js'
//          common.plugins.push(
//              new HtmlWebpackPlugin({
//                filename: 'index.html',
//                template: './pages/index.html',
//                chunks: ['index'],
//              })
//          );
//        }
//
//    Note that argv.mode will be set to either 'production', 'development',
//    or '' depending on the --mode flag passed to the webpack cli.
//
// You do not need to add any of the plugins or loaders used here to your
// local package.json, on the other hand, if you add new loaders or plugins
// in your local project then you should 'npm add' them to your local
// package.json.
//
//     build:
//        npx webpack --mode=development
//
//     release:
//        npx webpack --mode=production
//
const { glob } = require("glob");
const path = require("path");
const fs = require("fs");
const childProcess = require("child_process");
const { basename, join } = require("path");
const CleanWebpackPlugin = require("clean-webpack-plugin");
const MiniCssExtractPlugin = require("mini-css-extract-plugin");
const HtmlWebpackPlugin = require("html-webpack-plugin");
const CopyWebpackPlugin = require("copy-webpack-plugin");

const minifyOptions = {
  caseSensitive: true,
  collapseBooleanAttributes: true,
  collapseWhitespace: true,
  // this handles CSS minification in the .js files. For options
  // involving minifying .[s]css files, see ./**/postcss.config.js
  minifyCSS: true,
  minifyJS: true,
  minifyURLS: true,
  removeOptionalTags: true,
  removeRedundantAttributes: true,
  removeScriptTypeAttributes: true,
  removeStyleLinkTypeAttributes: true,
};

/**
 * Uses `luci-auth info` to fetch email address used in the live_demo mode.
 *
 * This is helpful as the email is used for some UI actions like creating
 * a termination task.
 *
 * Function will **crash** entire program if it fails for whatever reason.
 *
 * @returns email address of logged in luci according to luci-auth
 */
function findLiveDemoEmail() {
  const infoResult = childProcess.spawnSync("luci-auth", ["info"]);
  if (infoResult.status != 0) {
    console.error(`Error retrieving luci-auth info.

You may need to run \`luci-auth login\`

stdout: ${infoResult.stdout}
stderr: ${infoResult.stderr}
exit_code: ${infoResult.status}`);
    process.exit(1);
  }
  let email = "";
  try {
    /* The output of `luci-auth info` call will have the following format:
     *
     * Logged in as jonahhooper@google.com.
     *
     * OAuth token details:
     *   Client ID: xxxxx
     *   Scopes:
     *     https://www.googleapis.com/auth/userinfo.email
     *     openid
     *
     * We extract the email address from the first line.
     **/
    email = infoResult.stdout
      .toString()
      .split("\n")[0]
      .split(" ")[3]
      .slice(0, -1);
  } catch (e) {
    console.error(`Failed to parse email from luci-auth info.
stdout: ${infoResult.stdout}
error: ${e}`);
    process.exit(1);
  }
  return email;
}

/**
 * Uses `luci-auth token` to fetch credentials to be used in the live_demo mode.
 * Function will **crash** entire program if it fails for whatever reason.
 *
 * @returns token where token is an oauth access token derived from luci-auth.
 */
function findLiveDemoToken() {
  // The default scope for both of these calls is
  // https://www.googleapis.com/auth/userinfo.email
  const tokenResult = childProcess.spawnSync("luci-auth", [
    "token",
    "-lifetime",
    "20m",
  ]);
  if (tokenResult.status != 0) {
    console.error(`Error retrieving luci-auth token ${tokenResult.error}
stdout: ${tokenResult.stdout}
stderr: ${tokenResult.stderr}
exit_code: ${tokenResult.status}`);
    process.exit(1);
  }
  return tokenResult.stdout.toString();
}

function demoFinder(dir, webpackConfig, demoType) {
  // Look at all sub-directories of dir and if a directory contains
  // both a -demo.html and -demo.js file then add the corresponding
  // entry points and Html plugins to the config.

  // Find all the dirs below 'dir'.
  const isDir = (filename) => fs.lstatSync(filename).isDirectory();
  const moduleDir = path.resolve(dir, "modules");
  const dirs = fs
    .readdirSync(moduleDir)
    .map((name) => join(moduleDir, name))
    .filter(isDir);

  if (demoType !== "demo" && demoType !== "live") {
    throw new Error("Only 'demo' and 'live' are valid values for demo type");
  }
  const htmlSuffix = `-${demoType}.html`;
  const jsSuffix = `-${demoType}.js`;
  dirs.forEach((d) => {
    // Look for both a *-demo.js and *-demo.html file in the directory.
    const files = fs.readdirSync(d);
    let demoHTML = "";
    let demoJS = "";
    files.forEach((file) => {
      if (file.endsWith(htmlSuffix)) {
        if (!!demoHTML) {
          throw new Error(
            "Only one -demo.html file is allowed per directory: " + file
          );
        }
        demoHTML = file;
      }
      if (file.endsWith(jsSuffix)) {
        if (demoJS != "") {
          throw new Error(
            "Only one -demo.js file is allowed per directory: " + file
          );
        }
        demoJS = file;
      }
    });
    if (!!demoJS && !!demoHTML) {
      const name = basename(d);
      webpackConfig.entry[name] = join(d, demoJS);
      webpackConfig.plugins.push(
        new HtmlWebpackPlugin({
          filename: name + ".html",
          template: join(d, demoHTML),
          chunks: [name],
        })
      );
    } else if (!!demoJS || !!demoHTML) {
      console.log(
        "WARNING: An element needs both a *-demo.js and a *-demo.html file."
      );
    }
  });

  return webpackConfig;
}

/* A function that will look at all subdirectories of 'dir'/pages
 * and adds entries for each page it finds there.
 *
 * Presumes that each page will have both a JS and an HTML file.
 *
 *    pages/
 *      index.js
 *      index.html
 *      search.js
 *      search.html
 *      ....
 *
 * The function will find those files and do the equivalent
 * of the following to the webpack_config:
 *
 *      webpack_config.entry.['index'] = './pages/index/js';
 *      webpack_config.plugins.push(
 *        new HtmlWebpackPlugin({
 *          filename: 'index.html',
 *          template: './pages/index.html',
 *          chunks: ['index'],
 *        }),
 *      );
 *
 */
function pageFinder(dir, webpackConfig, minifyOutput) {
  // Look at all sub-directories of dir and if a directory contains
  // both a -demo.html and -demo.js file then add the corresponding
  // entry points and Html plugins to the config.

  // Find all the dirs below 'dir'.
  const pagesDir = path.resolve(dir, "pages");
  // Look for all *.js files, for each one look for a matching .html file.
  // Emit into config.
  //
  const pagesJS = glob.sync(pagesDir + "/*.js");

  pagesJS.forEach((pageJS) => {
    // Look for both a <filename>.js and <filename>.html file in the directory.
    // Strip off ".js" from end and replace with ".html".
    const pageHTML = pageJS.replace(/\.js$/, ".html");
    if (!fs.existsSync(pageHTML)) {
      console.log("WARNING: A page needs both a *.js and a *.html file.");
      return;
    }

    const baseHTML = basename(pageHTML);
    const name = basename(pageJS, ".js");
    webpackConfig.entry[name] = pageJS;
    const opts = {
      filename: baseHTML,
      template: pageHTML,
      chunks: [name],
    };
    if (minifyOutput) {
      opts.minify = minifyOptions;
    }
    webpackConfig.plugins.push(new HtmlWebpackPlugin(opts));
  });

  return webpackConfig;
}

module.exports = (env, argv) => {
  // The postcss config file must be named postcss.config.js, so we store the
  // different configs in different dirs.
  const dirname = __dirname;
  const prefix = argv.mode === "production" ? "prod" : "dev";
  // This file handles minification, auto-prefixing, etc.
  const postCssConfig = path.resolve(__dirname, prefix, "postcss.config.js");
  let common = {
    entry: {
      // Users of webpack.common must fill in the entry point(s).
    },
    output: {
      path: path.resolve(dirname, "dist"),
      filename: "[name]-bundle.js?[chunkhash]",
    },
    devServer: {
      static: {
        directory: path.join(__dirname, "dist"),
      },
    },
    module: {
      rules: [
        {
          test: /\.[s]?css$/,
          use: [
            {
              loader: MiniCssExtractPlugin.loader,
              options: {},
            },
            {
              loader: "css-loader",
              options: {
                importLoaders: 2, // postcss-loader and sass-loader.
              },
            },
            {
              loader: "postcss-loader",
              options: {
                postcssOptions: require(postCssConfig),
              },
            },
            {
              loader: "sass-loader",
              options: {
                implementation: require("sass"),
                sassOptions: {
                  includePaths: [__dirname],
                },
              },
            },
          ],
        },
        {
          test: /\.html$/,
          loader: "html-loader",
        },
      ],
    },
    plugins: [
      new MiniCssExtractPlugin({
        filename: "[name]-bundle.css",
      }),
      new CleanWebpackPlugin(["dist"], {
        root: path.resolve(dirname),
      }),
      // Users of pulito can append any plugins they want, but they
      // need to make sure they installed them in their project via npm.
    ],
  };
  common = pageFinder(dirname, common, argv.mode === "production");
  if (argv.mode !== "production") {
    const demoType = env.demo_type === "live" ? "live" : "demo";
    common = demoFinder(dirname, common, demoType);
    common.devtool = "eval-source-map";
    if (demoType === "live") {
      const email = findLiveDemoEmail();
      common.devServer.proxy = [
        {
          changeOrigin: true,
          context: ["/prpc/", "/_ah/"],
          target: "https://chromium-swarm-dev.appspot.com/",
        },
        {
          changeOrigin: true,
          context: ["/auth/openid/state"],
          target: "https://chromium-swarm-dev.appspot.com/",
          bypass: function (_req, res, _proxyOptions) {
            // The first call to `luci-auth token` will create the initial token
            // subsequent calls don't do a network call since the token is
            // already cached.
            const token = findLiveDemoToken();
            res.send({
              email: email,
              accessToken: token,
            });
          },
        },
      ];
    }
  }

  // Make all CSS/JS files appear at the /newres location.
  common.output.publicPath = "/newres/";
  if (argv.mode === "production") {
    common.module.rules.push({
      test: /.js$/,
      use: "html-template-minifier-webpack",
    });
  }
  common.plugins.push(
    new CopyWebpackPlugin({
      patterns: [
        "node_modules/@webcomponents/custom-elements/custom-elements.min.js",
      ],
    })
  );

  return common;
};
