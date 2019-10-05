package exe

import "compress/zlib"

type config struct {
	zlibLevel int
}

// Option is a type that allows you to modify the behavior of Run.
//
// See With* methods for available Options.
type Option func(*config)

// WithZlibCompression returns an Option; If unspecified, no compression will be
// applied to the outgoing Build.proto stream. Otherwise zlib compression at
// `level` will be used.
//
// level is capped between NoCompression and BestCompression.
//
// If level is NoCompression, it's the same as not specifying this option (i.e.
// no zlib wrapper will be used at all).
func WithZlibCompression(level int) Option {
	if level <= 0 {
		return nil
	}
	if level > zlib.BestCompression {
		level = zlib.BestCompression
	}
	return func(c *config) {
		c.zlibLevel = level
	}
}
