package player

import "testing"

func TestSourceFormatLabel(t *testing.T) {
	cases := []struct {
		in   SourceFormat
		want string
	}{
		{SourceFormat{}, ""},
		{SourceFormat{Codec: "flac", Bits: 24, SampleRate: 96000, Lossless: true}, "FLAC 24/96kHz"},
		{SourceFormat{Codec: "flac", Bits: 16, SampleRate: 44100, Lossless: true}, "FLAC 16/44.1kHz"},
		{SourceFormat{Codec: "flac", SampleRate: 48000, Lossless: true}, "FLAC 48kHz"},
		{SourceFormat{Codec: "aac", SampleRate: 44100}, "AAC 44.1kHz lossy"},
	}
	for _, c := range cases {
		if got := c.in.Label(); got != c.want {
			t.Errorf("%+v: got %q, want %q", c.in, got, c.want)
		}
	}
}
