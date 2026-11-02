package stream

// Format is an output encoding for the group stream.
type Format struct {
	Name        string
	Ext         string
	ContentType string
	Args        []string // ffmpeg output arguments
	// HeaderLen reports how long the stream header is, once enough bytes are known. Listeners that join
	// later are sent the header first. Nil for formats whose frames can be decoded on their own.
	HeaderLen func(b []byte) (n int, ok bool)
}

// Formats are offered per group, so different speakers' buffering behaviour can be compared.
var Formats = []Format{
	{"mp3-192", "mp3", "audio/mpeg", []string{"-c:a", "libmp3lame", "-b:a", "192k", "-f", "mp3"}, nil},
	{"mp3-128", "mp3", "audio/mpeg", []string{"-c:a", "libmp3lame", "-b:a", "128k", "-f", "mp3"}, nil},
	{"mp3-320", "mp3", "audio/mpeg", []string{"-c:a", "libmp3lame", "-b:a", "320k", "-f", "mp3"}, nil},
	{"aac", "aac", "audio/aac", []string{"-c:a", "aac", "-b:a", "192k", "-f", "adts"}, nil},
	{"flac", "flac", "audio/flac", []string{"-c:a", "flac", "-f", "flac"}, flacHeaderLen},
	{"wav", "wav", "audio/wav", []string{"-c:a", "pcm_s16le", "-f", "wav"}, wavHeaderLen},
}

// FormatByName returns the named format, or the default (first) one.
func FormatByName(name string) Format {
	for _, f := range Formats {
		if f.Name == name {
			return f
		}
	}
	return Formats[0]
}

// FormatByExt returns the format whose URL extension is ext ("mp3" gives the default MP3 format).
func FormatByExt(ext string) Format {
	for _, f := range Formats {
		if f.Ext == ext {
			return f
		}
	}
	return Formats[0]
}

// flacHeaderLen walks the metadata blocks after the "fLaC" marker.
func flacHeaderLen(b []byte) (int, bool) {
	if len(b) < 4 {
		return 0, false
	}
	pos := 4
	for {
		if len(b) < pos+4 {
			return 0, false
		}
		last := b[pos]&0x80 != 0
		pos += 4 + int(b[pos+1])<<16 + int(b[pos+2])<<8 + int(b[pos+3])
		if last {
			return pos, len(b) >= pos
		}
	}
}

// wavHeaderLen walks the RIFF chunks up to the start of the "data" chunk.
func wavHeaderLen(b []byte) (int, bool) {
	pos := 12
	for {
		if len(b) < pos+8 {
			return 0, false
		}
		if string(b[pos:pos+4]) == "data" {
			return pos + 8, true
		}
		pos += 8 + int(b[pos+4]) | int(b[pos+5])<<8 | int(b[pos+6])<<16 | int(b[pos+7])<<24
	}
}
