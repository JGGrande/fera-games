package feramusicas

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Song é uma música cadastrada.
type Song struct {
	ID       string // gerado pelo Catalog no Add (no Firebase vira o doc ID)
	Name     string // o que o jogador precisa acertar, ex.: "Queen - Bohemian Rhapsody"
	Category string // texto livre; comparação sem diferenciar caixa
	URL      string // link colado no cadastro
	VideoID  string // 11 caracteres extraídos do link; chave de unicidade
	// Start é de onde o trecho começa no vídeo (0 = do início). Serve para
	// pular introduções longas ou silêncio, que deixariam o round 1 mudo.
	Start time.Duration
}

// Catalog é onde as músicas ficam guardadas. Hoje só existe MemoryCatalog;
// trocar por Firebase é escrever outra implementação (todos os métodos já
// recebem context e devolvem erro), sem mexer na engine nem na TUI.
type Catalog interface {
	List(ctx context.Context) ([]Song, error)
	ByCategory(ctx context.Context, category string) ([]Song, error)
	// Categories são as categorias distintas das músicas, em ordem alfabética.
	Categories(ctx context.Context) ([]string, error)
	// Add valida (ver PrepareSong), rejeita VideoID repetido com ErrDuplicate
	// e devolve a música com ID preenchido.
	Add(ctx context.Context, s Song) (Song, error)
	// Update troca os dados da música id, validados como no Add. Um link que
	// já é de outra música dá ErrDuplicate; id inexistente, ErrNotFound.
	// Devolve a música como ficou: o ID pode mudar onde ele é o próprio
	// VideoID (Firestore) e o link foi trocado.
	Update(ctx context.Context, id string, s Song) (Song, error)
	Remove(ctx context.Context, id string) error
}

var (
	// ErrInvalid agrupa os erros de validação (veja ValidationError).
	ErrInvalid = errors.New("dados inválidos")
	// ErrDuplicate é um VideoID que já está no catálogo.
	ErrDuplicate = errors.New("essa música já está cadastrada")
	// ErrNotFound é um ID inexistente no Remove.
	ErrNotFound = errors.New("música não encontrada")
	// ErrEmptyCatalog é devolvido quando não há nenhuma música para jogar.
	ErrEmptyCatalog = errors.New("o catálogo está vazio")
	// ErrEmptyCategory é uma categoria sem músicas.
	ErrEmptyCategory = errors.New("a categoria não tem músicas")
)

// Limites dos campos do cadastro.
const (
	MaxNameLen     = 120
	MaxCategoryLen = 40
)

// ValidationError diz qual campo do cadastro está errado. Is(ErrInvalid) é
// verdadeiro; Error() é a frase para mostrar ao usuário.
type ValidationError struct {
	Field string // "name", "category" ou "url"
	Msg   string
}

func (e *ValidationError) Error() string { return e.Msg }
func (e *ValidationError) Unwrap() error { return ErrInvalid }

// Nomes dos campos em ValidationError.Field.
const (
	FieldName     = "name"
	FieldCategory = "category"
	FieldURL      = "url"
	FieldStart    = "start"
)

// failing é o catálogo usado quando o de verdade não pôde ser aberto (ex.:
// credenciais do Firebase inválidas): toda operação devolve o motivo, e a tela
// mostra o erro em vez de um catálogo vazio enganoso.
type failing struct{ err error }

func (f failing) List(context.Context) ([]Song, error)               { return nil, f.err }
func (f failing) ByCategory(context.Context, string) ([]Song, error) { return nil, f.err }
func (f failing) Categories(context.Context) ([]string, error)       { return nil, f.err }
func (f failing) Add(context.Context, Song) (Song, error)            { return Song{}, f.err }
func (f failing) Update(context.Context, string, Song) (Song, error) { return Song{}, f.err }
func (f failing) Remove(context.Context, string) error               { return f.err }

// FailingCatalog devolve um Catalog que sempre falha com err.
func FailingCatalog(err error) Catalog { return failing{err} }

// CleanText apara e colapsa espaços repetidos.
func CleanText(s string) string { return strings.Join(strings.Fields(s), " ") }

// CategoryKey é a chave de comparação de uma categoria (sem caixa).
func CategoryKey(category string) string { return strings.ToLower(CleanText(category)) }

// PrepareSong valida os campos do cadastro e devolve a música normalizada,
// com VideoID derivado do link. Toda implementação de Catalog chama isto no Add.
func PrepareSong(s Song) (Song, error) {
	s.Name = CleanText(s.Name)
	s.Category = CleanText(s.Category)
	s.URL = strings.TrimSpace(s.URL)
	switch {
	case s.Name == "":
		return Song{}, &ValidationError{FieldName, "informe o nome da música"}
	case len([]rune(s.Name)) > MaxNameLen:
		return Song{}, &ValidationError{FieldName, fmt.Sprintf("nome com mais de %d caracteres", MaxNameLen)}
	case s.Category == "":
		return Song{}, &ValidationError{FieldCategory, "informe a categoria"}
	case len([]rune(s.Category)) > MaxCategoryLen:
		return Song{}, &ValidationError{FieldCategory, fmt.Sprintf("categoria com mais de %d caracteres", MaxCategoryLen)}
	}
	id, err := ParseVideoID(s.URL)
	if err != nil {
		return Song{}, err
	}
	s.VideoID = id
	switch {
	case s.Start < 0:
		return Song{}, &ValidationError{FieldStart, "o início não pode ser negativo"}
	case s.Start > MaxStart:
		return Song{}, &ValidationError{FieldStart, fmt.Sprintf("início acima de %s", FormatStart(MaxStart))}
	}
	s.Start = s.Start.Truncate(time.Millisecond)
	return s, nil
}
