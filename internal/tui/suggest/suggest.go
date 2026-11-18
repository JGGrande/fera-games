// Package suggest ordena sugestões de autocomplete para as telas de jogo.
package suggest

import (
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"
)

// Match ordena: primeiro quem contém o texto literalmente (o mais curto
// antes, que costuma ser o mais específico), depois o fuzzy para completar.
func Match(q string, titles []string, limit int) []string {
	lq := strings.ToLower(q)
	var exact []string
	for _, t := range titles {
		if strings.Contains(strings.ToLower(t), lq) {
			exact = append(exact, t)
		}
	}
	sort.SliceStable(exact, func(i, j int) bool { return len(exact[i]) < len(exact[j]) })
	out := exact[:min(len(exact), limit)]
	if len(out) < limit {
		seen := make(map[string]bool, len(out))
		for _, t := range out {
			seen[t] = true
		}
		for _, f := range fuzzy.Find(q, titles) {
			if len(out) >= limit {
				break
			}
			if !seen[f.Str] {
				out = append(out, f.Str)
			}
		}
	}
	return out
}
