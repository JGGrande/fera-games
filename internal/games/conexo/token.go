package conexo

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// TokenEnv é a variável de ambiente com um token do App Check já obtido.
const TokenEnv = "FERA_CONEXO_APPCHECK"

var (
	// ErrNoToken indica que não há token válido: é preciso colar um.
	ErrNoToken = errors.New("falta o token do App Check do Conexo")
	// ErrBadToken é um texto que não parece um JWT do App Check.
	ErrBadToken = errors.New("token do App Check inválido")
)

// TokenStore guarda o JWT do App Check (vale 7 dias). Só o navegador
// consegue gerá-lo (reCAPTCHA Enterprise), então o jogador cola o token da
// própria sessão. Ordem de busca: colado nesta execução, TokenEnv, arquivo
// em Path. Um TokenStore nil não tem token.
type TokenStore struct {
	Path string           // vazio = só memória
	Now  func() time.Time // nil = time.Now

	mu     sync.Mutex
	pasted string
}

func (s *TokenStore) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

// Get devolve o primeiro token ainda não expirado, ou "".
func (s *TokenStore) Get() string {
	tok, _ := s.current()
	return tok
}

// Expiry é quando o token atual expira.
func (s *TokenStore) Expiry() (time.Time, bool) {
	tok, exp := s.current()
	return exp, tok != ""
}

func (s *TokenStore) current() (string, time.Time) {
	if s == nil {
		return "", time.Time{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	candidates := []string{s.pasted, os.Getenv(TokenEnv)}
	if s.Path != "" {
		if data, err := os.ReadFile(s.Path); err == nil {
			candidates = append(candidates, string(data))
		}
	}
	for _, tok := range candidates {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if exp, err := ParseToken(tok); err == nil && s.now().Before(exp) {
			return tok, exp
		}
	}
	return "", time.Time{}
}

// Set valida e guarda um token colado: o JWT puro ou qualquer texto que o
// contenha, como o registro do IndexedDB copiado do DevTools
// ({compositeKey: …, value: {token: …}}). Grava em Path com 0600.
func (s *TokenStore) Set(raw string) (time.Time, error) {
	if s == nil {
		return time.Time{}, errors.New("conexo: sem onde guardar o token")
	}
	tok := extractToken(raw)
	exp, err := ParseToken(tok)
	if err != nil {
		return time.Time{}, err
	}
	if !s.now().Before(exp) {
		return exp, fmt.Errorf("%w: expirou em %s", ErrBadToken, exp.Local().Format("02/01 15:04"))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Path != "" {
		if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
			return exp, err
		}
		if err := os.WriteFile(s.Path, []byte(tok+"\n"), 0o600); err != nil {
			return exp, err
		}
	}
	s.pasted = tok
	return exp, nil
}

// jwtRe acha um JWT (header JSON em base64url sempre começa com "eyJ").
var jwtRe = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)

// extractToken tira o JWT do texto colado; sem um, devolve o texto limpo
// para ParseToken explicar o erro.
func extractToken(raw string) string {
	if tok := jwtRe.FindString(raw); tok != "" {
		return tok
	}
	return strings.Trim(raw, "\"' \t\r\n")
}

// ParseToken lê o exp do JWT sem validar a assinatura (quem valida é o
// Firestore; aqui só serve para avisar antes de expirar).
func ParseToken(tok string) (time.Time, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return time.Time{}, fmt.Errorf("%w: não achei um JWT (eyJ…)", ErrBadToken)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: payload não é base64url", ErrBadToken)
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, fmt.Errorf("%w: payload sem exp", ErrBadToken)
	}
	return time.Unix(claims.Exp, 0), nil
}
