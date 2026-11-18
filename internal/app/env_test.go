package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fera-games/internal/games/feramusicas"
	"fera-games/internal/games/feramusicas/firestorecatalog"
)

func TestLoadEnvPrioridades(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "primeiro.env")
	second := filepath.Join(dir, "segundo.env")
	_ = os.WriteFile(first, []byte("FERA_T_A=do-primeiro\n"), 0o600)
	_ = os.WriteFile(second, []byte("FERA_T_A=do-segundo\nFERA_T_B=so-no-segundo\nFERA_T_C=arquivo\n"), 0o600)
	t.Setenv("FERA_T_C", "ambiente") // já definida: o arquivo não sobrescreve
	t.Cleanup(func() { _ = os.Unsetenv("FERA_T_A"); _ = os.Unsetenv("FERA_T_B") })

	loaded, err := LoadEnv(filepath.Join(dir, "nao-existe.env"), first, second)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 {
		t.Fatalf("loaded = %v; arquivo ausente deveria ser ignorado", loaded)
	}
	for k, want := range map[string]string{"FERA_T_A": "do-primeiro", "FERA_T_B": "so-no-segundo", "FERA_T_C": "ambiente"} {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, quero %q", k, got, want)
		}
	}
}

func TestLoadEnvChavePrivadaEntreAspas(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	_ = os.WriteFile(f, []byte("FERA_T_KEY=\"-----BEGIN PRIVATE KEY-----\\nABC\\n-----END PRIVATE KEY-----\\n\"\n"), 0o600)
	t.Cleanup(func() { _ = os.Unsetenv("FERA_T_KEY") })
	if _, err := LoadEnv(f); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("FERA_T_KEY"); !strings.Contains(got, "-----\nABC\n-----") {
		t.Fatalf("chave = %q; entre aspas duplas, \\n vira quebra de linha", got)
	}
}

func TestLoadEnvArquivoInvalido(t *testing.T) {
	f := filepath.Join(t.TempDir(), ".env")
	_ = os.WriteFile(f, []byte("FERA_T_X=\"sem fechar aspas\n"), 0o600)
	if _, err := LoadEnv(f); err == nil || !strings.Contains(err.Error(), f) {
		t.Fatalf("err = %v; quero erro citando o arquivo", err)
	}
}

func TestEnvFiles(t *testing.T) {
	got := EnvFiles("/dados")
	if len(got) != 2 || got[0] != ".env" || got[1] != filepath.Join("/dados", ".env") {
		t.Fatalf("EnvFiles = %v", got)
	}
	if got := EnvFiles(""); len(got) != 1 {
		t.Fatalf("EnvFiles sem pasta de dados = %v", got)
	}
}

func getenv(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestCatalogoSemFirebaseFicaEmMemoriaComAviso(t *testing.T) {
	c, notice := feraMusicasCatalog(getenv(nil))
	if _, ok := c.(*feramusicas.MemoryCatalog); !ok || !strings.Contains(notice, "só em memória") {
		t.Fatalf("catálogo %T, aviso %q", c, notice)
	}
}

func TestCatalogoComConfiguracaoInvalidaMostraOErro(t *testing.T) {
	// E-mail sem chave: não pode cair na memória em silêncio.
	c, notice := feraMusicasCatalog(getenv(map[string]string{
		firestorecatalog.EnvProjectID: "p", firestorecatalog.EnvClientEmail: "a@b",
	}))
	_, err := c.List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "configuração do Firebase") || notice != "" {
		t.Fatalf("err = %v, aviso %q", err, notice)
	}
	if _, err := c.Add(context.Background(), feramusicas.Song{}); err == nil || errors.Is(err, feramusicas.ErrInvalid) {
		t.Fatalf("Add deveria devolver o erro de configuração, não validar: %v", err)
	}
}

func TestCatalogoComFirebaseUsaOFirestore(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	key := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	c, notice := feraMusicasCatalog(getenv(map[string]string{
		firestorecatalog.EnvProjectID:   "meu-projeto",
		firestorecatalog.EnvClientEmail: "bot@meu-projeto.iam.gserviceaccount.com",
		firestorecatalog.EnvPrivateKey:  key,
	}))
	fc, ok := c.(*firestorecatalog.Catalog)
	if !ok || notice != "" {
		t.Fatalf("catálogo %T, aviso %q; quero o Firestore sem aviso", c, notice)
	}
	_ = fc.Close()
}
