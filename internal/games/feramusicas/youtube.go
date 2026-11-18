package feramusicas

import (
	"net/url"
	"regexp"
	"strings"
)

var videoIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// youtubeHosts são os hosts aceitos (já sem "www.").
var youtubeHosts = map[string]bool{
	"youtube.com":          true,
	"m.youtube.com":        true,
	"music.youtube.com":    true,
	"youtube-nocookie.com": true,
	"youtu.be":             true,
}

// ParseVideoID extrai o ID de 11 caracteres de um link do YouTube:
// watch?v=, youtu.be/, /shorts/, /embed/, /live/ e /v/, inclusive em
// m.youtube.com e music.youtube.com. Links sem "https://" também valem.
func ParseVideoID(link string) (string, error) {
	bad := &ValidationError{FieldURL, "link do YouTube inválido"}
	link = strings.TrimSpace(link)
	if link == "" {
		return "", &ValidationError{FieldURL, "informe o link do YouTube"}
	}
	if !strings.Contains(link, "://") {
		link = "https://" + link
	}
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return "", bad
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if !youtubeHosts[host] {
		return "", bad
	}

	var id string
	if host == "youtu.be" {
		id = firstSegment(u.Path)
	} else {
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		switch {
		case len(segs) == 1 && segs[0] == "watch":
			id = u.Query().Get("v")
		case len(segs) == 2 && (segs[0] == "shorts" || segs[0] == "embed" || segs[0] == "live" || segs[0] == "v"):
			id = segs[1]
		}
	}
	if !videoIDRe.MatchString(id) {
		return "", bad
	}
	return id, nil
}

func firstSegment(path string) string {
	seg, _, _ := strings.Cut(strings.Trim(path, "/"), "/")
	return seg
}

// WatchURL é o link canônico usado para baixar: montado só a partir do ID já
// validado, sem parâmetros extras (playlist, tempo etc.) vindos do cadastro.
func WatchURL(videoID string) string { return "https://www.youtube.com/watch?v=" + videoID }
