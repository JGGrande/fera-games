package har

import (
	"strings"
	"testing"
)

const sample = `{"log":{"entries":[
 {"_resourceType":"fetch","request":{"method":"GET","url":"https://api.example-game.app/daily?day=1491&v=2","headers":[{"name":"Authorization","value":"Bearer abcdefghijklmnop"}]},
  "response":{"status":200,"headers":[{"name":"Content-Type","value":"application/json"}],"content":{"mimeType":"application/json","text":"{\"id\":1491}"}}},
 {"_resourceType":"fetch","request":{"method":"GET","url":"https://api.example-game.app/daily?day=1490","headers":[]},
  "response":{"status":304,"headers":[],"content":{"mimeType":"application/json","text":""}}},
 {"_resourceType":"media","request":{"method":"GET","url":"https://cdn.example-game.app/songs/abc/drums.mp3","headers":[{"name":"Range","value":"bytes=0-"}]},
  "response":{"status":206,"headers":[],"content":{"mimeType":"audio/mpeg"}}},
 {"_resourceType":"script","request":{"method":"GET","url":"https://example-game.app/static/js/main.123.js","headers":[]},
  "response":{"status":200,"headers":[],"content":{"mimeType":"application/javascript"}}},
 {"_resourceType":"script","request":{"method":"GET","url":"https://www.googletagmanager.com/gtag/js?id=G-1","headers":[]},
  "response":{"status":200,"headers":[],"content":{"mimeType":"application/javascript"}}},
 {"_resourceType":"image","request":{"method":"GET","url":"https://example-game.app/logo.png","headers":[]},
  "response":{"status":200,"headers":[],"content":{"mimeType":"image/png"}}}
]}}`

func TestGroup(t *testing.T) {
	f, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	eps := Group(f.Log.Entries)
	if len(eps) != 3 {
		t.Fatalf("esperava 3 endpoints (data, media, script), veio %d: %+v", len(eps), eps)
	}
	if eps[0].Kind != KindData || eps[0].Count != 2 || len(eps[0].Status) != 2 {
		t.Fatalf("endpoint de dados agrupado errado: %+v", eps[0])
	}
	if got := strings.Join(eps[0].Query, ","); got != "day,v" && got != "v,day" {
		t.Fatalf("query params = %q", got)
	}
	if eps[1].Kind != KindMedia || eps[2].Kind != KindScript {
		t.Fatalf("ordem/classificação errada: %s, %s", eps[1].Kind, eps[2].Kind)
	}
}

func TestInterestingMasksSecrets(t *testing.T) {
	hs := Interesting([]Header{
		{Name: "Authorization", Value: "Bearer abcdefghijklmnop"},
		{Name: "User-Agent", Value: "x"},
	})
	if len(hs) != 1 || strings.Contains(hs[0].Value, "ijklmnop") {
		t.Fatalf("header sensível não mascarado: %+v", hs)
	}
}
