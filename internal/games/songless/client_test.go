package songless

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"fera-games/internal/games/songless/fixture"
	"fera-games/internal/httpx"
)

// fakeServer imita a página, a API e o CDN.
type fakeServer struct {
	*httptest.Server
	page     string
	search   map[string][]byte
	guess    map[string][]byte // chave: cat/round/song ou skip
	audio    map[string][]byte
	hits     map[string]*atomic.Int32
	stats    []byte
	pageHits atomic.Int32
}

func readFile(t *testing.T, name string) []byte { return testdata(t, name) }

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()
	fs := &fakeServer{
		search: map[string][]byte{},
		guess:  map[string][]byte{},
		audio:  map[string][]byte{},
		hits:   map[string]*atomic.Int32{},
	}
	fs.page = fixture.PageHTML
	fs.search["marvin"] = readFile(t, "search-marvin.json")
	fs.guess["0/0/SEMMQgZhm--14_IC7XGtZ"] = readFile(t, "guess-correct.json")
	fs.guess["0/0/present"] = readFile(t, "guess-present.json")
	fs.guess["0/0/wrong"] = readFile(t, "guess-incorrect.json")
	fs.guess["0/0/skip"] = readFile(t, "guess-skip.json")
	fs.guess["1/4/skip"] = readFile(t, "guess-final-skip.json")
	fs.audio["https://cdn.lessgames.com/songs/A101.aac"] = readFile(t, "A101.aac")
	fs.stats = readFile(t, "stats-2026-09-27-0.json")
	for k := range fs.guess {
		fs.hits[k] = &atomic.Int32{}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /songless", func(w http.ResponseWriter, r *http.Request) {
		fs.pageHits.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(fs.page))
	})
	mux.HandleFunc("GET /songless/search", func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(r.URL.Query().Get("query"))
		if strings.Contains(q, "marvin") {
			_, _ = w.Write(fs.search["marvin"])
			return
		}
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("POST /songless/guess", func(w http.ResponseWriter, r *http.Request) {
		var v struct {
			ChallengeDate string  `json:"challengeDate"`
			CategoryID    int     `json:"categoryId"`
			RoundIndex    int     `json:"roundIndex"`
			SongID        *string `json:"songId"`
		}
		_ = jsonDecode(r, &v)
		if v.ChallengeDate == "2026-09-29" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"DATE_NOT_ALLOWED"}`))
			return
		}
		var key string
		// Mapeia IDs reais para as chaves das fixtures.
		if v.SongID != nil {
			switch *v.SongID {
			case "SEMMQgZhm--14_IC7XGtZ":
				key = "0/0/SEMMQgZhm--14_IC7XGtZ"
			case "Z-tYFxL9NVdKHg2yye5DB":
				key = "0/0/present"
			default:
				key = "0/0/wrong"
			}
		} else if v.CategoryID == 1 && v.RoundIndex == 4 {
			key = "1/4/skip"
		} else {
			key = "0/0/skip"
		}
		if h, ok := fs.hits[key]; ok {
			h.Add(1)
		}
		if body, ok := fs.guess[key]; ok {
			_, _ = w.Write(body)
			return
		}
		_, _ = w.Write([]byte(`{"state":"incorrect"}`))
	})
	mux.HandleFunc("GET /songless/stats", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fs.stats)
	})
	mux.HandleFunc("GET /songs/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/aac")
		_, _ = w.Write(fs.audio["https://cdn.lessgames.com/songs/A101.aac"])
	})
	fs.Server = httptest.NewServer(mux)
	t.Cleanup(fs.Close)
	return fs
}

func jsonDecode(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func newTestClient(t *testing.T, fs *fakeServer, cacheDir string) *Client {
	t.Helper()
	c := NewClient(httpx.New(5*time.Second), &httpx.Cache{Dir: cacheDir})
	// Reescreve os fileUrls do CDN para o httptest.
	c.Endpoints = Endpoints{Page: fs.URL + "/songless", API: fs.URL}
	return c
}

func TestClientDaily(t *testing.T) {
	fs := newFakeServer(t)
	c := newTestClient(t, fs, t.TempDir())
	d, err := c.Daily(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if d.Date != "2026-09-27" || len(d.Categories) != 3 {
		t.Fatalf("daily = %+v", d)
	}
	// Segunda chamada usa o cache da página (1 h).
	if _, err := c.Daily(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if n := fs.pageHits.Load(); n != 1 {
		t.Fatalf("página baixada %d vezes, quero 1", n)
	}
	for _, cat := range d.Categories {
		cat.FileURL = fs.URL + "/songs/A101.aac"
		rc, err := c.Audio(context.Background(), cat)
		if err != nil {
			t.Fatal(err)
		}
		rc.Close()
	}
}

func TestClientSearchGuessStats(t *testing.T) {
	fs := newFakeServer(t)
	c := newTestClient(t, fs, "")
	ctx := context.Background()

	entries, err := c.Search(ctx, "Marvin Gaye")
	if err != nil || len(entries) == 0 {
		t.Fatalf("busca: %v %+v", err, entries)
	}
	if entries[0].Artist != "Marvin Gaye" {
		t.Fatalf("primeiro = %+v", entries[0])
	}

	id := "SEMMQgZhm--14_IC7XGtZ"
	out, ans, err := c.Guess(ctx, Guess{Date: "2026-09-27", CategoryID: 0, RoundIndex: 0, SongID: &id})
	if err != nil || out != OutcomeCorrect || ans == nil || ans.Track != "Ain't No Mountain High Enough" {
		t.Fatalf("acerto: %v %+v %v", out, ans, err)
	}
	presentID := "Z-tYFxL9NVdKHg2yye5DB"
	if out, _, err := c.Guess(ctx, Guess{Date: "2026-09-27", CategoryID: 0, RoundIndex: 0, SongID: &presentID}); err != nil || out != OutcomePresent {
		t.Fatalf("present: %v %v", out, err)
	}
	wrong := "xxx"
	if out, _, err := c.Guess(ctx, Guess{Date: "2026-09-27", CategoryID: 0, RoundIndex: 0, SongID: &wrong}); err != nil || out != OutcomeIncorrect {
		t.Fatalf("erro: %v %v", out, err)
	}
	if out, _, err := c.Guess(ctx, Guess{Date: "2026-09-27", CategoryID: 0, RoundIndex: 0, SongID: nil}); err != nil || out != OutcomeSkipped {
		t.Fatalf("skip: %v %v", out, err)
	}
	if _, ans, err := c.Guess(ctx, Guess{Date: "2026-09-27", CategoryID: 1, RoundIndex: 4, SongID: nil}); err != nil || ans == nil {
		t.Fatalf("5º skip deveria revelar: %+v %v", ans, err)
	}

	if _, _, err := c.Guess(ctx, Guess{Date: "2026-09-29", CategoryID: 0, RoundIndex: 0}); !errors.Is(err, ErrNoPuzzle) {
		t.Fatalf("dia futuro: %v", err)
	}

	st, err := c.Stats(ctx, "2026-09-27", 0)
	if err != nil || len(st.Daily.Distribution) == 0 {
		t.Fatalf("stats: %+v %v", st, err)
	}
}
