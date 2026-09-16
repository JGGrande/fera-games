package main

import "testing"

func TestSniff(t *testing.T) {
	cases := map[string][]byte{
		"mp3 (com tag ID3)":  []byte("ID3\x04\x00"),
		"mp3 (frame MPEG)":   {0xFF, 0xFB, 0x90, 0x64},
		"aac (ADTS)":         {0xFF, 0xF1, 0x50, 0x80},
		"ogg (vorbis)":       []byte("OggS\x00\x02........vorbis"),
		"opus (ogg)":         []byte("OggS\x00\x02..........................OpusHead"),
		"mp4/m4a (ftyp M4A)": append([]byte{0, 0, 0, 0x20}, []byte("ftypM4A isom")...),
		"wav":                []byte("RIFF\x24\x08\x00\x00WAVEfmt "),
		"json":               []byte("  {\"id\": 1}"),
		"vazio":              {},
	}
	for want, in := range cases {
		if got := Sniff(in); got != want {
			t.Errorf("Sniff(%q) = %q, want %q", in, got, want)
		}
	}
}
