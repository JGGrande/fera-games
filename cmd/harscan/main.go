// Command harscan lê um HAR exportado do DevTools jogando o Bandle e gera:
//   - um relatório markdown com endpoints, headers e formatos (rascunho do protocolo);
//   - os corpos JSON salvos em testdata/ para testes com httptest.
//
// Uso:
//
//	go run ./cmd/harscan -har bandle.har -out testdata/bandle -report docs/bandle-har-report.md
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"fera-games/internal/har"
)

func main() {
	harPath := flag.String("har", "", "arquivo .har exportado do DevTools (obrigatório)")
	outDir := flag.String("out", "testdata/bandle", "pasta para salvar os corpos JSON")
	report := flag.String("report", "docs/bandle-har-report.md", "relatório markdown gerado")
	flag.Parse()
	if *harPath == "" {
		flag.Usage()
		os.Exit(2)
	}

	fh, err := os.Open(*harPath)
	if err != nil {
		log.Fatal(err)
	}
	defer fh.Close()
	f, err := har.Parse(fh)
	if err != nil {
		log.Fatal(err)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}
	eps := har.Group(f.Log.Entries)

	var md bytes.Buffer
	fmt.Fprintf(&md, "# Relatório do HAR: %s\n\n", filepath.Base(*harPath))
	fmt.Fprintf(&md, "%d requisições no HAR, %d endpoints relevantes (ruído de analytics/anúncios/imagens removido).\n\n",
		len(f.Log.Entries), len(eps))
	md.WriteString("| Tipo | Método | Host | Path | Query | Status | MIME | Vezes |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, ep := range eps {
		fmt.Fprintf(&md, "| %s | %s | %s | `%s` | %s | %v | %s | %d |\n",
			ep.Kind, ep.Method, ep.Host, ep.Path, strings.Join(ep.Query, ", "), ep.Status, ep.Mime, ep.Count)
	}

	saved := 0
	for i, ep := range eps {
		if ep.Kind != har.KindData {
			continue
		}
		fmt.Fprintf(&md, "\n## %s %s%s\n\n", ep.Method, ep.Host, ep.Path)
		md.WriteString("Headers de requisição relevantes:\n\n")
		writeHeaders(&md, har.Interesting(ep.Example.Request.Headers))
		md.WriteString("\nHeaders de resposta relevantes:\n\n")
		writeHeaders(&md, har.Interesting(ep.Example.Response.Headers))
		if pd := ep.Example.Request.PostData; pd != nil && pd.Text != "" {
			fmt.Fprintf(&md, "\nCorpo enviado (%s):\n\n```\n%s\n```\n", pd.MimeType, truncate(pd.Text, 800))
		}

		body, err := ep.Example.Body()
		if err != nil || len(body) == 0 {
			md.WriteString("\n_Corpo não incluído no HAR (exporte com \"Save all as HAR with content\")._\n")
			continue
		}
		name := fmt.Sprintf("%02d-%s.json", i, slug(ep.Host+ep.Path))
		dst := filepath.Join(*outDir, name)
		if pretty, ok := prettyJSON(body); ok {
			body = pretty
		}
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			log.Fatal(err)
		}
		saved++
		fmt.Fprintf(&md, "\nCorpo salvo em `%s`. Estrutura:\n\n```\n%s```\n", dst, shape(body))
	}

	md.WriteString("\n## Mídia (áudio)\n\nSó URLs e headers — os arquivos não são salvos. Rode `go run ./cmd/probe -url <URL>` para detectar o formato.\n\n")
	for _, ep := range eps {
		if ep.Kind != har.KindMedia {
			continue
		}
		fmt.Fprintf(&md, "- `%s%s` (%s, status %v, %d vezes)\n", ep.Host, ep.Path, ep.Mime, ep.Status, ep.Count)
	}

	md.WriteString("\n## Pistas nos bundles JS\n\nBaixe os scripts listados acima e rode:\n\n")
	md.WriteString("```\ngrep -oE '(https?:)?//[a-zA-Z0-9./_-]+|fetch\\([^)]{0,120}|firebase[a-zA-Z.]*|\\.(mp3|ogg|m4a)' main*.js | sort | uniq -c | sort -rn | head -50\n```\n")

	if err := os.MkdirAll(filepath.Dir(*report), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*report, md.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("relatório: %s | %d endpoints | %d corpos JSON salvos em %s\n", *report, len(eps), saved, *outDir)
}

func writeHeaders(md *bytes.Buffer, hs []har.Header) {
	if len(hs) == 0 {
		md.WriteString("- (nenhum)\n")
		return
	}
	for _, h := range hs {
		fmt.Fprintf(md, "- `%s: %s`\n", h.Name, truncate(h.Value, 120))
	}
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 60 {
		s = s[len(s)-60:]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func prettyJSON(b []byte) ([]byte, bool) {
	var out bytes.Buffer
	if err := json.Indent(&out, b, "", "  "); err != nil {
		return nil, false
	}
	return out.Bytes(), true
}

// shape descreve a estrutura de um JSON (chaves e tipos), sem valores,
// para documentar o payload sem expor a resposta do dia.
func shape(b []byte) string {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return "(não é JSON)\n"
	}
	var sb strings.Builder
	walk(&sb, v, "", 0)
	return sb.String()
}

func walk(sb *strings.Builder, v any, name string, depth int) {
	if depth > 6 {
		return
	}
	pad := strings.Repeat("  ", depth)
	switch t := v.(type) {
	case map[string]any:
		fmt.Fprintf(sb, "%s%s{object, %d chaves}\n", pad, label(name), len(t))
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			walk(sb, t[k], k, depth+1)
		}
	case []any:
		fmt.Fprintf(sb, "%s%s[array, %d itens]\n", pad, label(name), len(t))
		if len(t) > 0 {
			walk(sb, t[0], "[0]", depth+1)
		}
	case string:
		fmt.Fprintf(sb, "%s%sstring(%d)\n", pad, label(name), len(t))
	case float64:
		fmt.Fprintf(sb, "%s%snumber\n", pad, label(name))
	case bool:
		fmt.Fprintf(sb, "%s%sbool\n", pad, label(name))
	case nil:
		fmt.Fprintf(sb, "%s%snull\n", pad, label(name))
	}
}

func label(name string) string {
	if name == "" {
		return ""
	}
	return name + ": "
}
