package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"

	"fera-games/internal/games/feramusicas"
	"fera-games/internal/games/feramusicas/firestorecatalog"
)

// EnvFiles são os .env lidos, em ordem de prioridade: o da pasta onde o
// programa roda e o da pasta de dados (ex.: ~/.config/fera-games/.env), que
// serve quando o binário é chamado de qualquer lugar.
func EnvFiles(dataDir string) []string {
	files := []string{".env"}
	if dataDir != "" {
		files = append(files, filepath.Join(dataDir, ".env"))
	}
	return files
}

// LoadEnv carrega os .env que existirem e devolve quais foram lidos.
// Variáveis já definidas no ambiente valem mais que as dos arquivos, e o
// primeiro arquivo vale mais que o segundo (godotenv não sobrescreve).
func LoadEnv(files ...string) ([]string, error) {
	var loaded []string
	for _, f := range files {
		if _, err := os.Stat(f); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err := godotenv.Load(f); err != nil {
			return loaded, fmt.Errorf("ler %s: %w", f, err)
		}
		loaded = append(loaded, f)
	}
	return loaded, nil
}

// memoryNotice aparece na tela do catálogo quando não há Firebase configurado.
const memoryNotice = "sem Firebase configurado (.env): o catálogo fica só em memória e some ao fechar"

// feraMusicasCatalog escolhe o catálogo do Fera Músicas a partir do ambiente:
// Firestore quando FIREBASE_PROJECT_ID está definido, memória quando não está.
// Configuração inválida não cai para a memória em silêncio: vira um catálogo
// que falha, e a tela mostra o motivo.
func feraMusicasCatalog(getenv func(string) string) (feramusicas.Catalog, string) {
	cfg, err := firestorecatalog.ConfigFromEnv(getenv)
	if errors.Is(err, firestorecatalog.ErrNotConfigured) {
		return feramusicas.NewMemoryCatalog(), memoryNotice
	}
	if err != nil {
		return feramusicas.FailingCatalog(fmt.Errorf("configuração do Firebase: %w", err)), ""
	}
	// Background: o contexto do NewClient vale para a vida do cliente.
	c, err := firestorecatalog.New(context.Background(), cfg)
	if err != nil {
		return feramusicas.FailingCatalog(err), ""
	}
	return c, ""
}
