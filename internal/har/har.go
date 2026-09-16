// Package har lê arquivos HAR exportados pelo DevTools e classifica as
// requisições relevantes para mapear o protocolo de um jogo.
package har

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
)

type File struct {
	Log struct {
		Entries []Entry `json:"entries"`
	} `json:"log"`
}

type Entry struct {
	StartedDateTime string   `json:"startedDateTime"`
	ResourceType    string   `json:"_resourceType"`
	Request         Request  `json:"request"`
	Response        Response `json:"response"`
}

type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Request struct {
	Method   string   `json:"method"`
	URL      string   `json:"url"`
	Headers  []Header `json:"headers"`
	PostData *struct {
		MimeType string `json:"mimeType"`
		Text     string `json:"text"`
	} `json:"postData,omitempty"`
}

type Response struct {
	Status  int      `json:"status"`
	Headers []Header `json:"headers"`
	Content struct {
		Size     int64  `json:"size"`
		MimeType string `json:"mimeType"`
		Text     string `json:"text"`
		Encoding string `json:"encoding"`
	} `json:"content"`
}

// Kind agrupa requisições pelo papel no jogo.
type Kind string

const (
	KindData   Kind = "data"   // JSON/XHR/fetch: puzzle, catálogo, validação
	KindMedia  Kind = "media"  // áudio
	KindScript Kind = "script" // bundles JS (para buscar endpoints)
	KindNoise  Kind = "noise"  // analytics, anúncios, fontes, imagens
	KindOther  Kind = "other"
)

// noiseHosts são hosts de terceiros que não fazem parte do protocolo do jogo.
var noiseHosts = []string{
	"google-analytics.com", "googletagmanager.com", "doubleclick.net",
	"googlesyndication.com", "nitropay.com", "fonts.googleapis.com",
	"fonts.gstatic.com", "adservice.google", "facebook.", "clarity.ms",
	"amazon-adsystem.com", "sentry.io",
}

func Parse(r io.Reader) (*File, error) {
	var f File
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return nil, fmt.Errorf("har inválido: %w", err)
	}
	return &f, nil
}

func (e Entry) Host() string {
	u, err := url.Parse(e.Request.URL)
	if err != nil {
		return ""
	}
	return u.Host
}

func (e Entry) Classify() Kind {
	host := e.Host()
	for _, n := range noiseHosts {
		if strings.Contains(host, n) {
			return KindNoise
		}
	}
	mime := strings.ToLower(e.Response.Content.MimeType)
	path := strings.ToLower(e.Request.URL)
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	switch {
	case e.ResourceType == "media" || strings.HasPrefix(mime, "audio/") ||
		hasAnySuffix(path, ".mp3", ".ogg", ".m4a", ".aac", ".wav", ".opus", ".webm", ".flac"):
		return KindMedia
	case strings.Contains(mime, "json") || e.ResourceType == "xhr" || e.ResourceType == "fetch":
		return KindData
	case strings.Contains(mime, "javascript") || e.ResourceType == "script" || strings.HasSuffix(path, ".js"):
		return KindScript
	case strings.HasPrefix(mime, "image/") || strings.HasPrefix(mime, "font/") ||
		strings.Contains(mime, "css") || e.ResourceType == "image" || e.ResourceType == "font" ||
		e.ResourceType == "stylesheet":
		return KindNoise
	}
	return KindOther
}

// Body devolve o corpo da resposta decodificado, se o HAR o incluiu.
func (e Entry) Body() ([]byte, error) {
	c := e.Response.Content
	if c.Text == "" {
		return nil, nil
	}
	if c.Encoding == "base64" {
		return base64.StdEncoding.DecodeString(c.Text)
	}
	return []byte(c.Text), nil
}

// interestingHeaders são os headers que indicam auth, cache ou CORS.
var interestingHeaders = map[string]bool{
	"authorization": true, "cookie": true, "x-api-key": true, "x-firebase-appcheck": true,
	"x-client-version": true, "x-goog-api-key": true, "apikey": true, "origin": true,
	"referer": true, "set-cookie": true, "access-control-allow-origin": true,
	"cache-control": true, "etag": true, "content-type": true, "content-length": true,
	"accept-ranges": true, "x-ratelimit-limit": true, "x-ratelimit-remaining": true, "retry-after": true,
}

// Interesting filtra headers relevantes, mascarando valores sensíveis.
func Interesting(hs []Header) []Header {
	var out []Header
	for _, h := range hs {
		name := strings.ToLower(h.Name)
		if !interestingHeaders[name] {
			continue
		}
		v := h.Value
		switch name {
		case "authorization", "cookie", "set-cookie", "x-api-key", "x-goog-api-key", "apikey", "x-firebase-appcheck":
			v = mask(v)
		}
		out = append(out, Header{Name: name, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Endpoint é uma URL normalizada (sem query) com as ocorrências agrupadas.
type Endpoint struct {
	Kind    Kind
	Method  string
	Host    string
	Path    string
	Query   []string // nomes dos parâmetros vistos
	Status  []int
	Mime    string
	Count   int
	Example Entry
}

// Group agrupa entradas por (kind, método, host, path), ignorando ruído.
func Group(entries []Entry) []Endpoint {
	idx := map[string]*Endpoint{}
	var order []string
	for _, e := range entries {
		k := e.Classify()
		if k == KindNoise {
			continue
		}
		u, err := url.Parse(e.Request.URL)
		if err != nil {
			continue
		}
		key := string(k) + " " + e.Request.Method + " " + u.Host + u.Path
		ep, ok := idx[key]
		if !ok {
			ep = &Endpoint{Kind: k, Method: e.Request.Method, Host: u.Host, Path: u.Path,
				Mime: e.Response.Content.MimeType, Example: e}
			idx[key] = ep
			order = append(order, key)
		}
		ep.Count++
		ep.Status = appendUniqueInt(ep.Status, e.Response.Status)
		for q := range u.Query() {
			ep.Query = appendUniqueStr(ep.Query, q)
		}
	}
	out := make([]Endpoint, 0, len(order))
	rank := map[Kind]int{KindData: 0, KindMedia: 1, KindScript: 2, KindOther: 3}
	for _, k := range order {
		out = append(out, *idx[k])
	}
	sort.SliceStable(out, func(i, j int) bool { return rank[out[i].Kind] < rank[out[j].Kind] })
	return out
}

func hasAnySuffix(s string, suffixes ...string) bool {
	for _, x := range suffixes {
		if strings.HasSuffix(s, x) {
			return true
		}
	}
	return false
}

func mask(v string) string {
	if len(v) <= 8 {
		return "****"
	}
	return v[:4] + "…(" + fmt.Sprint(len(v)) + " chars)"
}

func appendUniqueInt(s []int, v int) []int {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func appendUniqueStr(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
