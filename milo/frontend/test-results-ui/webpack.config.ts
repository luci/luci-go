import path from 'path';
import webpack from 'webpack';

const config: webpack.Configuration = {
  mode: 'development',
  entry: './src/index.ts',
  output: {
    path: path.resolve(
        __dirname, '../appengine/test-results-ui/static/dist/scripts/'),
    filename: 'index.js',
  },
  module: {
    rules: [
      {
        test: /\.ts$/,
        use: 'ts-loader',
        exclude: /node_modules/,
      },
    ],
  },
  resolve: {
    extensions: ['.js', '.ts'],
  },
  devServer: {
    contentBase: path.join(__dirname, '../appengine/test-results-ui/'),
    publicPath: '/static/dist/scripts/',
    historyApiFallback: true,
  },
};

// Default export is required by webpack.
// tslint:disable-next-line: no-default-export
export default config;
