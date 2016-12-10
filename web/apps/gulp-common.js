/*
  Copyright 2016 The LUCI Authors. All rights reserved.
  Use of this source code is governed under the Apache License, Version 2.0
  that can be found in the LICENSE file.

  This document has been largely derived from the Polymer Starter Kit:
  https://github.com/PolymerElements/polymer-starter-kit
*/

'use strict';

var path = require('path');
var argv = require('yargs').argv;

var exports = module.exports = {}
exports.base = path.join(__dirname, '..');
exports.out = (argv.out || exports.base);
exports.plugins = require('gulp-load-plugins')({
  config: path.join(exports.base, 'package.json'),
});

// Include Gulp & tools we'll use
var $ = exports.plugins;
var browserSync = require('browser-sync');
var debug = require('gulp-debug');
var del = require('del');
var foreach = require('gulp-foreach');
var fs = require('fs');
var glob = require('glob-all');
var historyApiFallback = require('connect-history-api-fallback');
var hyd = require('hydrolysis');
var merge = require('merge-stream');
var pathExists = require('path-exists');
var reload = browserSync.reload;
var rename = require('gulp-rename');
var requireDir = require('require-dir');
var runSequence = require('run-sequence');
var through = require('through2');
var ts = require('gulp-typescript');


var AUTOPREFIXER_BROWSERS = [
  'ie >= 10',
  'ie_mob >= 10',
  'ff >= 30',
  'chrome >= 34',
  'safari >= 7',
  'opera >= 23',
  'ios >= 7',
  'android >= 4.4',
  'bb >= 10'
];

exports.setup = function(gulp, config) {
  var APP = path.basename(config.dir);
  var BUILD = path.join('.tmp', 'build');
  var DIST = path.join(exports.out, 'dist', APP);

  var layout = {
    app: APP,
    distPath: DIST,

    // NOTE: Takes vararg via "arguments".
    dist: function() {
      return extendPath(DIST).apply(null, arguments);
    },
  };

  var extendPath = function() {
    var base = [].slice.call(arguments);
    return function() {
      // Simple case: only base, no additional elements.
      if (base.length === 1 && arguments.length === 0) {
        return base[0];
      }

      var parts = base.concat();
      parts.push.apply(parts, arguments)
      return path.join.apply(null, parts);
    };
  };

  var styleTask = function(stylesPath, srcs) {
    return gulp.src(srcs.map(function(src) {
        return path.join(BUILD, stylesPath, src);
      }))
      .pipe($.autoprefixer(AUTOPREFIXER_BROWSERS))
      .pipe(gulp.dest('.tmp/' + stylesPath))
      .pipe($.minifyCss())
      .pipe(gulp.dest(layout.dist(stylesPath)))
      .pipe($.size({title: stylesPath}));
  };

  var imageOptimizeTask = function(src, dest) {
    return gulp.src(path.join(BUILD, src))
      .pipe($.imagemin({
        progressive: true,
        interlaced: true
      }))
      .pipe(gulp.dest(dest))
      .pipe($.size({title: 'images'}));
  };

  var optimizeHtmlTask = function(src, dest) {
    var assets = $.useref.assets({
      searchPath: ['.tmp', config.dir]
    });

    return gulp.src(src, {cwd: BUILD})
      .pipe(assets)
      // Concatenate and minify JavaScript
      .pipe($.if('*.js', $.uglify({
        preserveComments: 'some'
      })))
      // Concatenate and minify styles
      // In case you are still using useref build blocks
      .pipe($.if('*.css', $.minifyCss()))
      .pipe(assets.restore())
      .pipe($.useref())
      // Minify any HTML
      .pipe($.if('*.html', $.minifyHtml({
        quotes: true,
        empty: true,
        spare: true
      })))
      // Output files
      .pipe(gulp.dest(dest))
      .pipe($.size({
        title: 'html'
      }));
  };

  gulp.task('build', function() {
    // Copy application directories.
    var app = gulp.src([
        '**',
        '!inc',
    ]).pipe(gulp.dest(BUILD));

    var inc = gulp.src('inc/**/*', { cwd: exports.base })
        .pipe(gulp.dest(path.join(BUILD, 'inc')));
    return merge(app, inc);
  });

  // Compile and automatically prefix stylesheets
  gulp.task('styles', ['build'], function() {
    return styleTask('styles', ['**/*.css']);
  });

  gulp.task('elements', ['build'], function() {
    return styleTask('elements', ['**/*.css']);
  });

  // Optimize images
  gulp.task('images', ['build'], function() {
    return imageOptimizeTask('images/**/*', layout.dist('images'));
  });

  // Transpiles "inc/*/*.ts" and deposits the result alongside their source
  // "ts" files.
  gulp.task('tsinline', function() {
    // Transpile each TypeScript module independently into JavaScript in the
    // BUILD directory.
    var incDir = path.join(exports.base, 'inc')
    var tsconfigPath = path.join(incDir, 'tsconfig.json');
    var tsProj = ts.createProject(tsconfigPath, {
      typeRoots: [path.join(exports.base, 'node_modules', '@types')],
    });
    return gulp.src('*/*.ts', { cwd: incDir })
        .pipe(tsProj())
        .pipe(gulp.dest(incDir));
  });

  var tsCompileSingle = function(tsconfigPath, dest) {
    // Transpile each TypeScript module independently into JavaScript in the
    // BUILD directory.
    return pathExists(tsconfigPath).then( (exists) => {
      if ( !exists ) {
        return;
      }

      var tsProj = ts.createProject(tsconfigPath, {
        target: 'ES5', // Vulcanize can't handle ES6 ATM.
        removeComments: true,
        module: "amd",
        outFile: 'ts-app.js',
        typeRoots: [path.join(exports.base, 'node_modules', '@types')],
      });
      return gulp.src('main.ts', { cwd: path.join(BUILD, 'scripts-ts') })
          .pipe(tsProj())
          .pipe(rename(function(fpath) {
            fpath.dirname = ".";
          }))
          .pipe(gulp.dest(dest));
    } );
  };

  // Builds the project's "ts-app.js" into the project directory.
  gulp.task('tsproject', function() {
    return tsCompileSingle(
        path.join(BUILD, 'scripts-ts', 'tsconfig.json'),
        path.join('scripts') );
  });

  gulp.task('ts', ['build'], function() {
    return tsCompileSingle(
        path.join(BUILD, 'scripts-ts', 'tsconfig.json'),
        path.join(path.join(BUILD, 'scripts')) );
  });

  // Copy all files at the root level (app)
  gulp.task('copy', ['build'], function() {
    // Application files.
    var app = gulp.src([
      '*',
      '!inc',
      '!test',
      '!elements',
      '!inc/bower_components',
      '!cache-config.json',
      '!**/.DS_Store',
      '!gulpfile.js',
      '!package.json',
      '!scripts-ts',
    ], {
      cwd: BUILD,
    }).pipe(gulp.dest(layout.dist()));

    // Copy over only the bower_components we need
    // These are things which cannot be vulcanized
    var webcomponentsjs = gulp.src([
      'inc/bower_components/webcomponentsjs/webcomponents-lite.min.js',
    ], {
      cwd: BUILD,
    }).pipe(gulp.dest(layout.dist('inc/bower_components/webcomponentsjs/')));

    var requirejs = gulp.src([
      'inc/bower_components/requirejs/require.js',
    ], {
      cwd: BUILD,
    }).pipe(gulp.dest(layout.dist('inc/bower_components/requirejs/')));

    var includes = (config.includes) ? (config.includes(gulp, layout)) : ([]);
    return merge(app, includes, webcomponentsjs, requirejs)
      .pipe($.size({
        title: 'copy'
      }));
  });

  // Copy web fonts to dist
  gulp.task('fonts', ['build'], function() {
    return gulp.src(['fonts/**'], {cwd: BUILD})
      .pipe(gulp.dest(layout.dist('fonts')))
      .pipe($.size({
        title: 'fonts'
      }));
  });

  // Scan your HTML for assets & optimize them
  gulp.task('html', ['build'], function() {
    return optimizeHtmlTask(
      ['**/*.html', '!{elements,test,inc}/**/*.html'],
      layout.dist());
  });

  // Vulcanize granular configuration
  gulp.task('vulcanize', ['build', 'ts'], function() {
    var fsResolver = hyd.FSResolver
    return gulp.src('elements/elements.html', {cwd: BUILD})
      .pipe($.vulcanize({
        stripComments: true,
        inlineCss: true,
        inlineScripts: true,
      }))
      .pipe(gulp.dest(layout.dist('elements')))
      .pipe($.size({title: 'vulcanize'}));
  });

  // Clean output directory
  gulp.task('clean', function() {
    var dist = layout.dist();
    var remove = ['.tmp', path.join(dist, '*')];
    var keep = '!'+path.join(dist, '.keep');
    return del(remove.concat(keep), {force: true, dot:true});
  });

  // Watch files for changes & reload
  gulp.task('servebuild', ['build'], function() {
    gulp.watch([
    '**',
    '!.tmp',
  ], ['build']);
  });

  // Watch files for changes & reload
  gulp.task('serve', ['default'], function() {
    browserSync({
      port: 8000,
      ui: {
        port: 8080,
      },
      notify: false,
      logPrefix: 'PSK',
      snippetOptions: {
        rule: {
          match: '<span id="browser-sync-binding"></span>',
          fn: function(snippet) {
            return snippet;
          }
        }
      },
      // Run as an https by uncommenting 'https: true'
      // Note: this uses an unsigned certificate which on first access
      //       will present a certificate warning in the browser.
      // https: true,
      server: {
        baseDir: [BUILD],
        middleware: [historyApiFallback()]
      }
    });

    gulp.watch(['**/*.html'], ['html', reload]);
    gulp.watch(['styles/**/*.css'], ['styles', reload]);
    gulp.watch(['elements/**/*.css'], ['elements', reload]);
    gulp.watch(['images/**/*'], ['build', reload]);
    gulp.watch(['inc/**/*'], { cwd: exports.base }, ['ts', reload]);
  });

  // Build and serve the output from the dist build
  gulp.task('serve:dist', ['default'], function() {
    browserSync({
      port: 8000,
      ui: {
        port: 8080,
      },
      notify: false,
      logPrefix: 'PSK',
      snippetOptions: {
        rule: {
          match: '<span id="browser-sync-binding"></span>',
          fn: function(snippet) {
            return snippet;
          }
        }
      },
      // Run as an https by uncommenting 'https: true'
      // Note: this uses an unsigned certificate which on first access
      //       will present a certificate warning in the browser.
      // https: true,
      server: layout.dist(),
      middleware: [historyApiFallback()]
    });
  });

  // Build production files, the default task
  gulp.task('default', ['clean'], function(cb) {
    runSequence(
      ['ts', 'copy', 'styles', 'images', 'fonts', 'html'],
      'vulcanize',
      cb);
  });
};

require('es6-promise').polyfill();

// Load custom tasks from the `tasks` directory
try {
  require('require-dir')('tasks');
} catch (err) {}
