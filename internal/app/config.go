package app

import (
	"os"
	"path/filepath"
)

// Config são as opções de linha de comando e caminhos do app.
type Config struct {
	Mute    bool   // não tocar áudio
	DataDir string // estado das partidas e cache (padrão: UserConfigDir/fera-games)
}

// DefaultConfig resolve o diretório de dados; devolve erro só se nem o
// diretório de configuração do usuário existir.
func DefaultConfig() (Config, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return Config{}, err
	}
	return Config{DataDir: filepath.Join(base, "fera-games")}, nil
}
