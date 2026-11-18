package feramusicas

import (
	"errors"
	"testing"
)

func TestParseVideoID(t *testing.T) {
	const id = "fJ9rUzIMcZQ"
	valid := []string{
		"https://www.youtube.com/watch?v=" + id,
		"https://youtube.com/watch?v=" + id + "&list=PL123&t=42s",
		"http://youtube.com/watch?v=" + id,
		"youtube.com/watch?v=" + id,
		"www.youtube.com/watch?v=" + id,
		"  https://m.youtube.com/watch?v=" + id + "  ",
		"https://music.youtube.com/watch?v=" + id + "&si=abc",
		"https://youtu.be/" + id,
		"https://youtu.be/" + id + "?si=abc&t=3",
		"youtu.be/" + id,
		"https://www.youtube.com/shorts/" + id,
		"https://www.youtube.com/embed/" + id,
		"https://www.youtube.com/live/" + id + "?feature=share",
		"https://www.youtube-nocookie.com/embed/" + id,
	}
	for _, in := range valid {
		got, err := ParseVideoID(in)
		if err != nil || got != id {
			t.Errorf("ParseVideoID(%q) = %q, %v; quero %q", in, got, err, id)
		}
	}

	invalid := []string{
		"",
		"   ",
		id, // ID solto não é link
		"https://www.youtube.com/watch",
		"https://www.youtube.com/watch?v=curto",
		"https://www.youtube.com/watch?v=" + id + "x", // 12 caracteres
		"https://www.youtube.com/watch?v=" + id[:10] + "!",
		"https://www.youtube.com/playlist?list=PL123",
		"https://www.youtube.com/@canal",
		"https://youtu.be/",
		"https://vimeo.com/123456789",
		"https://evil.com/watch?v=" + id,
		"https://youtube.com.evil.com/watch?v=" + id,
		"https://notyoutube.com/watch?v=" + id,
		"ftp://youtube.com/watch?v=" + id,
		"javascript:alert(1)",
		"%%%",
	}
	for _, in := range invalid {
		_, err := ParseVideoID(in)
		var ve *ValidationError
		if !errors.As(err, &ve) || ve.Field != FieldURL || !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseVideoID(%q) = %v; quero ValidationError no campo url", in, err)
		}
	}
}

func TestWatchURLIgnoraParametrosDoCadastro(t *testing.T) {
	id, err := ParseVideoID("https://www.youtube.com/watch?v=fJ9rUzIMcZQ&list=PL123&index=4")
	if err != nil {
		t.Fatal(err)
	}
	if got := WatchURL(id); got != "https://www.youtube.com/watch?v=fJ9rUzIMcZQ" {
		t.Fatalf("WatchURL = %q", got)
	}
}
