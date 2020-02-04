import path from 'path';
import webpack from 'webpack';
var TypedCssPlugin = require('./plugins/typed-constructable-style-sheet-plugin');

const config: webpack.Configuration = {
  mode: 'development',
  entry: './src/index.ts',
  output: {
    path: path.resolve(__dirname, 'static/dist/scripts'),
    filename: 'index.js'
  },
  module: {
    rules: [
      {
        enforce: 'pre',
        test: /\.css$/,
        use: [
          'constructable-style-sheet-loader',
          'to-string-loader',
          // "@teamsupercell/typings-for-css-modules-loader",
          {
            loader: 'css-loader',
          }
        ],
      },
      {
        test: /\.ts$/,
        use: 'ts-loader',
        exclude: /node_modules/,
      },
    ],
  },
  resolve: {
    extensions: ['.css', '.js', '.ts'],
  },
  resolveLoader: {
    modules: [
      'node_modules',
      path.resolve(__dirname, './loaders'),
    ]
  },
  plugins: [
    new TypedCssPlugin({globPattern: '**/*.css'})
  ],
  devServer: {
    contentBase: path.join(__dirname, 'static'),
    writeToDisk: true,
    historyApiFallback: true,
  }
};

export default config;
