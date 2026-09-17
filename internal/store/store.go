// Package store persiste as partidas de cada jogo em JSON, dentro do
// diretório de dados do usuário, e deriva estatísticas do histórico.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Match é o registro de uma partida. Não guarda a resposta: só o que o
// jogador fez. Games gravam aqui o próprio Snapshot serializado em Data.
type Match struct {
	Day      string          `json:"day"`    // YYYY-MM-DD, chave
	Number   int             `json:"number"` // número do puzzle
	Status   string          `json:"status"` // playing, won, lost
	Tries    int             `json:"tries"`  // tentativas usadas
	Data     json.RawMessage `json:"data"`   // estado específico do jogo (ex.: bandle.Snapshot)
	SavedAt  time.Time       `json:"savedAt"`
	Finished bool            `json:"finished"`
}

// Stats são as estatísticas mostradas no compartilhamento e no hub.
type Stats struct {
	Played, Won, CurrentStreak, MaxStreak int
}

// file é o conteúdo de <Dir>/<game>.json.
type file struct {
	Version int              `json:"version"`
	Matches map[string]Match `json:"matches"` // por dia
}

// Store grava um arquivo por jogo em Dir. Dir vazio desliga a persistência.
type Store struct {
	Dir string
}

// Load devolve a partida de um dia, se houver.
func (s *Store) Load(gameID, day string) (Match, bool, error) {
	f, err := s.read(gameID)
	if err != nil {
		return Match{}, false, err
	}
	m, ok := f.Matches[day]
	return m, ok, nil
}

// Save grava (ou substitui) a partida do dia.
func (s *Store) Save(gameID string, m Match) error {
	if s == nil || s.Dir == "" {
		return nil
	}
	f, err := s.read(gameID)
	if err != nil {
		return err
	}
	if m.SavedAt.IsZero() {
		m.SavedAt = time.Now()
	}
	f.Matches[m.Day] = m
	return s.write(gameID, f)
}

// Stats calcula as estatísticas a partir das partidas terminadas. A sequência
// conta dias consecutivos com vitória, terminando no último dia jogado (ou no
// dia anterior a hoje, se hoje ainda não acabou); um dia sem jogar a quebra.
func (s *Store) Stats(gameID string, today string) (Stats, error) {
	f, err := s.read(gameID)
	if err != nil {
		return Stats{}, err
	}
	days := make([]string, 0, len(f.Matches))
	for d, m := range f.Matches {
		if m.Finished {
			days = append(days, d)
		}
	}
	sort.Strings(days)

	var st Stats
	streak, prev := 0, ""
	for _, d := range days {
		m := f.Matches[d]
		st.Played++
		if m.Status == "won" {
			st.Won++
			if prev != "" && nextDay(prev) == d {
				streak++
			} else {
				streak = 1
			}
			st.MaxStreak = max(st.MaxStreak, streak)
		} else {
			streak = 0
		}
		prev = d
	}
	// A sequência atual só vale se o último dia jogado for hoje ou ontem.
	if prev == today || nextDay(prev) == today {
		st.CurrentStreak = streak
	}
	return st, nil
}

func nextDay(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

func (s *Store) path(gameID string) string {
	return filepath.Join(s.Dir, gameID+".json")
}

func (s *Store) read(gameID string) (file, error) {
	f := file{Version: 1, Matches: map[string]Match{}}
	if s == nil || s.Dir == "" {
		return f, nil
	}
	data, err := os.ReadFile(s.path(gameID))
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("%s corrompido: %w", s.path(gameID), err)
	}
	if f.Matches == nil {
		f.Matches = map[string]Match{}
	}
	return f, nil
}

func (s *Store) write(gameID string, f file) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, "."+gameID+"-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name()) // melhor esforço
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name()) // melhor esforço
		return err
	}
	return os.Rename(tmp.Name(), s.path(gameID))
}
