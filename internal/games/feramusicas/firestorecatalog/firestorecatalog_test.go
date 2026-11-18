package firestorecatalog

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"fera-games/internal/games/feramusicas"
)

const fakeKey = "-----BEGIN PRIVATE KEY-----\nMIIabc\n-----END PRIVATE KEY-----\n"

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestConfigFromEnv(t *testing.T) {
	if _, err := ConfigFromEnv(env(nil)); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("sem projeto: err = %v, quero ErrNotConfigured", err)
	}

	cfg, err := ConfigFromEnv(env(map[string]string{
		EnvProjectID:   " meu-projeto ",
		EnvClientEmail: "bot@meu-projeto.iam.gserviceaccount.com",
		// Como fica numa linha só do .env: quebras escapadas.
		EnvPrivateKey: `-----BEGIN PRIVATE KEY-----\nMIIabc\n-----END PRIVATE KEY-----\n`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProjectID != "meu-projeto" || cfg.Collection != DefaultCollection || cfg.Database != "" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.PrivateKey != strings.TrimSpace(fakeKey) {
		t.Fatalf("PrivateKey = %q; \\n escapado deveria virar quebra de linha", cfg.PrivateKey)
	}

	// Chave com quebras reais (godotenv já expande "\n" entre aspas duplas).
	if cfg, err := ConfigFromEnv(env(map[string]string{
		EnvProjectID: "p", EnvClientEmail: "a@b", EnvPrivateKey: fakeKey,
		EnvCollection: "outra", EnvDatabase: "jogos",
	})); err != nil || cfg.Collection != "outra" || cfg.Database != "jogos" {
		t.Fatalf("cfg = %+v, %v", cfg, err)
	}

	// Só o projeto: vale GOOGLE_APPLICATION_CREDENTIALS / emulador.
	if _, err := ConfigFromEnv(env(map[string]string{EnvProjectID: "p"})); err != nil {
		t.Fatalf("só o projeto deveria valer: %v", err)
	}

	bad := []map[string]string{
		{EnvProjectID: "p", EnvClientEmail: "a@b"},                            // falta a chave
		{EnvProjectID: "p", EnvPrivateKey: fakeKey},                           // falta o e-mail
		{EnvProjectID: "p", EnvClientEmail: "a@b", EnvPrivateKey: "senha123"}, // não é chave
		{EnvProjectID: "p", EnvCollection: "a/b"},                             // caminho, não coleção
	}
	for _, m := range bad {
		if _, err := ConfigFromEnv(env(m)); err == nil || errors.Is(err, ErrNotConfigured) {
			t.Errorf("ConfigFromEnv(%v) = %v; quero erro de configuração", m, err)
		}
	}
}

func TestCredentialsJSONEhUmaContaDeServico(t *testing.T) {
	cfg := Config{ProjectID: "p", ClientEmail: "a@b", PrivateKey: fakeKey}
	raw, err := cfg.credentialsJSON()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["type"] != "service_account" || got["project_id"] != "p" || got["client_email"] != "a@b" ||
		got["private_key"] != fakeKey || got["token_uri"] == "" {
		t.Fatalf("credenciais = %v", got)
	}
}

// realKey gera uma chave RSA PKCS#8 em PEM, como a do JSON de conta de serviço.
func realKey(t *testing.T) string {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func TestNewComChaveValidaNaoFalaComOServidor(t *testing.T) {
	// Abrir o cliente não depende de rede (o app abre mesmo offline); a
	// credencial só é usada na primeira operação.
	key := realKey(t)
	// Do jeito que vem do .env: numa linha só, com \n escapado.
	cfg, err := ConfigFromEnv(env(map[string]string{
		EnvProjectID: "p", EnvClientEmail: "bot@p.iam.gserviceaccount.com",
		EnvPrivateKey: strings.ReplaceAll(key, "\n", `\n`),
	}))
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New = %v", err)
	}
	_ = c.Close()
}

func TestNewComChaveIlegivelFalhaNaAbertura(t *testing.T) {
	_, err := New(context.Background(), Config{ProjectID: "p", Collection: DefaultCollection, ClientEmail: "a@b", PrivateKey: fakeKey})
	if err == nil || !strings.Contains(err.Error(), "abrir o Firestore") {
		t.Fatalf("New com chave lixo = %v; quero erro na abertura", err)
	}
}

func TestDocIdaEVolta(t *testing.T) {
	s := feramusicas.Song{ID: "fJ9rUzIMcZQ", Name: "Queen - Bohemian Rhapsody", Category: "Rock",
		URL: "https://youtu.be/fJ9rUzIMcZQ", VideoID: "fJ9rUzIMcZQ", Start: 90250 * time.Millisecond}
	d := toDoc(s)
	if d.CategoryKey != "rock" {
		t.Fatalf("CategoryKey = %q", d.CategoryKey)
	}
	got, ok := fromDoc("fJ9rUzIMcZQ", d)
	if !ok || got != s {
		t.Fatalf("fromDoc = %+v, %v; quero %+v", got, ok, s)
	}
}

func TestFromDocToleraDocumentosFeitosAMao(t *testing.T) {
	// Sem videoId: vale o ID do documento.
	if s, ok := fromDoc("fJ9rUzIMcZQ", doc{Name: " A ", Category: "Rock"}); !ok || s.VideoID != "fJ9rUzIMcZQ" || s.Name != "A" {
		t.Fatalf("sem videoId: %+v %v", s, ok)
	}
	for name, d := range map[string]doc{
		"sem nome":      {Category: "Rock", VideoID: "fJ9rUzIMcZQ"},
		"sem categoria": {Name: "A", VideoID: "fJ9rUzIMcZQ"},
		"id inválido":   {Name: "A", Category: "Rock", VideoID: "curto"},
	} {
		if _, ok := fromDoc("x", d); ok {
			t.Errorf("%s: deveria ser ignorado", name)
		}
	}
}

func TestStartFromToleraValoresImpossiveis(t *testing.T) {
	cases := map[float64]time.Duration{
		0: 0, -5: 0, math.NaN(): 0, math.Inf(1): 0, 1e9: 0,
		45: 45 * time.Second, 90.25: 90250 * time.Millisecond,
	}
	for in, want := range cases {
		if got := startFrom(in); got != want {
			t.Errorf("startFrom(%v) = %v, quero %v", in, got, want)
		}
	}
}

func TestMapErr(t *testing.T) {
	cases := []struct {
		code codes.Code
		want string
	}{
		{codes.Unauthenticated, "credenciais do Firebase recusadas"},
		{codes.PermissionDenied, "não tem acesso"},
		{codes.NotFound, "banco do Firestore não encontrado"},
		{codes.Unavailable, "sem conexão"},
		{codes.Internal, "firestore:"},
	}
	for _, tc := range cases {
		err := mapErr(status.Error(tc.code, "detalhe"))
		if err == nil || !strings.Contains(err.Error(), tc.want) || status.Code(errors.Unwrap(err)) != tc.code {
			t.Errorf("%v: %v", tc.code, err)
		}
	}
	if err := mapErr(context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("timeout: %v", err)
	}
	if mapErr(nil) != nil {
		t.Error("nil deveria continuar nil")
	}
}

// TestEmulador exercita o catálogo inteiro contra o emulador do Firestore.
// Fora da CI; rode com, por exemplo:
//
//	gcloud emulators firestore start --host-port=localhost:8086
//	FIRESTORE_EMULATOR_HOST=localhost:8086 go test ./internal/games/feramusicas/firestorecatalog/
func TestEmulador(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("defina FIRESTORE_EMULATOR_HOST para rodar contra o emulador do Firestore")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := Config{ProjectID: "fera-test", Collection: fmt.Sprintf("songs_%d", time.Now().UnixNano())}
	c, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	if cats, err := c.Categories(ctx); err != nil || len(cats) != 0 {
		t.Fatalf("coleção nova: %v %v", cats, err)
	}
	add := func(name, category, url string) feramusicas.Song {
		t.Helper()
		s, err := c.Add(ctx, feramusicas.Song{Name: name, Category: category, URL: url})
		if err != nil {
			t.Fatalf("Add(%q): %v", name, err)
		}
		return s
	}
	q := add("Queen - Bohemian Rhapsody", "Rock", "https://youtu.be/fJ9rUzIMcZQ")
	withStart, err := c.Add(ctx, feramusicas.Song{Name: "Com início", Category: "Intro", URL: "https://youtu.be/ccccccccccc", Start: 75 * time.Second})
	if err != nil || withStart.Start != 75*time.Second {
		t.Fatalf("Add com início = %+v, %v", withStart, err)
	}
	if err := c.Remove(ctx, withStart.ID); err != nil {
		t.Fatal(err)
	}
	if q.ID != "fJ9rUzIMcZQ" || q.VideoID != q.ID {
		t.Fatalf("ID = %q, quero o VideoID", q.ID)
	}
	time.Sleep(20 * time.Millisecond) // createdAt distinto
	add("Nirvana - Come As You Are", "rock", "https://www.youtube.com/watch?v=vabnZ9-ex7o")
	time.Sleep(20 * time.Millisecond)
	add("Michael Jackson - Billie Jean", "Pop", "https://music.youtube.com/watch?v=Zi_XLOBDo_Y")

	// O início sobrevive à ida e volta pelo Firestore.
	if _, err := c.Add(ctx, feramusicas.Song{Name: "Intro longa", Category: "Intro", URL: "https://youtu.be/ddddddddddd", Start: 62500 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	intro, _ := c.ByCategory(ctx, "Intro")
	if len(intro) != 1 || intro[0].Start != 62500*time.Millisecond {
		t.Fatalf("início lido do Firestore = %+v", intro)
	}
	if err := c.Remove(ctx, intro[0].ID); err != nil {
		t.Fatal(err)
	}

	all, err := c.List(ctx)
	if err != nil || len(all) != 3 || all[0].Name != "Queen - Bohemian Rhapsody" || all[2].Category != "Pop" {
		t.Fatalf("List (ordem de cadastro) = %+v, %v", all, err)
	}
	cats, _ := c.Categories(ctx)
	if strings.Join(cats, ",") != "Pop,Rock" {
		t.Fatalf("Categories = %v; Rock/rock é uma só, com a grafia da mais antiga", cats)
	}
	rock, _ := c.ByCategory(ctx, " ROCK ")
	if len(rock) != 2 {
		t.Fatalf("ByCategory = %+v", rock)
	}

	// Duplicado (mesmo vídeo por outro link) e inválido.
	if _, err := c.Add(ctx, feramusicas.Song{Name: "X", Category: "Pop", URL: "https://www.youtube.com/watch?v=fJ9rUzIMcZQ&t=3"}); !errors.Is(err, feramusicas.ErrDuplicate) {
		t.Fatalf("duplicado: %v", err)
	}
	if _, err := c.Add(ctx, feramusicas.Song{Name: "", Category: "Pop", URL: "https://youtu.be/aaaaaaaaaaa"}); !errors.Is(err, feramusicas.ErrInvalid) {
		t.Fatalf("inválido: %v", err)
	}

	// Dois cadastros simultâneos do mesmo vídeo: só um entra.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = c.Add(ctx, feramusicas.Song{Name: fmt.Sprint("corrida ", i), Category: "Pop", URL: "https://youtu.be/bbbbbbbbbbb"})
		}()
	}
	wg.Wait()
	if (errs[0] == nil) == (errs[1] == nil) {
		t.Fatalf("corrida: errs = %v; quero exatamente um ErrDuplicate", errs)
	}

	// Update com o mesmo link: muda os campos, mantém ID e ordem.
	q2, err := c.Update(ctx, q.ID, feramusicas.Song{Name: "Queen - Bohemian Rhapsody (ao vivo)", Category: "Rock", URL: "https://youtu.be/fJ9rUzIMcZQ", Start: 10 * time.Second})
	if err != nil || q2.ID != q.ID || q2.Start != 10*time.Second {
		t.Fatalf("Update mesmo link = %+v, %v", q2, err)
	}
	if all, _ := c.List(ctx); all[0].Name != "Queen - Bohemian Rhapsody (ao vivo)" {
		t.Fatalf("depois do Update, a ordem de cadastro deveria se manter: %+v", all)
	}
	// Update trocando o link: o documento muda de ID e continua na mesma posição.
	q3, err := c.Update(ctx, q.ID, feramusicas.Song{Name: "Queen - Radio Ga Ga", Category: "Rock", URL: "https://youtu.be/azdwsXLmrHE"})
	if err != nil || q3.ID != "azdwsXLmrHE" {
		t.Fatalf("Update trocando o link = %+v, %v", q3, err)
	}
	if all, _ := c.List(ctx); all[0].ID != "azdwsXLmrHE" {
		t.Fatalf("o documento novo deveria herdar o createdAt do antigo: %+v", all)
	}
	if _, err := c.Update(ctx, q.ID, feramusicas.Song{Name: "x", Category: "Rock", URL: "https://youtu.be/eeeeeeeeeee"}); !errors.Is(err, feramusicas.ErrNotFound) {
		t.Fatalf("o ID antigo não existe mais: %v", err)
	}
	if _, err := c.Update(ctx, q3.ID, feramusicas.Song{Name: "x", Category: "Rock", URL: "https://www.youtube.com/watch?v=vabnZ9-ex7o"}); !errors.Is(err, feramusicas.ErrDuplicate) {
		t.Fatalf("link de outra música: %v", err)
	}
	q = q3

	if err := c.Remove(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.Remove(ctx, q.ID); !errors.Is(err, feramusicas.ErrNotFound) {
		t.Fatalf("Remove repetido = %v", err)
	}
	if err := c.Remove(ctx, "a/b"); !errors.Is(err, feramusicas.ErrNotFound) {
		t.Fatalf("Remove com barra = %v", err)
	}
	if all, _ := c.List(ctx); len(all) != 3 { // 2 restantes + 1 da corrida
		t.Fatalf("List após Remove = %+v", all)
	}

	// O jogo de ponta a ponta sobre o Firestore: mesma música do dia que o
	// catálogo em memória escolheria com as mesmas músicas.
	g := feramusicas.NewGame(c, nil)
	s, err := g.Start(ctx, time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC), "Pop")
	if err != nil {
		t.Fatal(err)
	}
	if s.Category() != "Pop" || len(s.Names()) != 3 {
		t.Fatalf("sessão: categoria %q, nomes %v", s.Category(), s.Names())
	}
}
