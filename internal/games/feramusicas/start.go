package feramusicas

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// MaxStart é o maior início aceito no cadastro. Vídeos de música raramente
// passam de uma hora; o teto só barra erro de digitação (ex.: 9000 por 90).
const MaxStart = 12 * time.Hour

// ParseStart lê o início de uma música como o cadastro aceita: vazio (= 0),
// segundos ("45", "45.5" ou "45,5"), "m:ss" ("1:30") ou "h:mm:ss" ("1:02:03").
func ParseStart(text string) (time.Duration, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}
	bad := &ValidationError{FieldStart, "início inválido: use segundos (45) ou minutos:segundos (1:30)"}
	parts := strings.Split(text, ":")
	if len(parts) > 3 {
		return 0, bad
	}
	var total float64
	for i, p := range parts {
		last := i == len(parts)-1
		var v float64
		if last {
			f, err := strconv.ParseFloat(strings.Replace(p, ",", ".", 1), 64)
			if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				return 0, bad
			}
			v = f
		} else {
			n, err := strconv.Atoi(p)
			if err != nil {
				return 0, bad
			}
			v = float64(n)
		}
		if v < 0 || strings.HasPrefix(p, "+") {
			return 0, bad
		}
		// Minutos e segundos depois de um ":" vão de 0 a 59 (1:75 é erro).
		if i > 0 && v >= 60 {
			return 0, bad
		}
		total = total*60 + v
	}
	d := time.Duration(math.Round(total*1000)) * time.Millisecond
	if d > MaxStart {
		return 0, &ValidationError{FieldStart, fmt.Sprintf("início acima de %s", FormatStart(MaxStart))}
	}
	return d, nil
}

// FormatStart mostra o início como o cadastro aceita: "" para 0, "m:ss" até
// uma hora e "h:mm:ss" daí em diante (com décimos quando houver).
func FormatStart(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	ms := d.Milliseconds()
	h, ms := ms/3_600_000, ms%3_600_000
	m, ms := ms/60_000, ms%60_000
	s, ms := ms/1000, ms%1000
	sec := fmt.Sprintf("%02d", s)
	if ms > 0 {
		sec += strings.TrimRight(fmt.Sprintf(".%03d", ms), "0")
	}
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%s", h, m, sec)
	}
	return fmt.Sprintf("%d:%s", m, sec)
}
