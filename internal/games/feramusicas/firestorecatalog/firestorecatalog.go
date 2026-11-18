// Package firestorecatalog guarda o catálogo do Fera Músicas no Cloud Firestore
// do Firebase, compartilhado por todo mundo que joga: com o mesmo catálogo, a
// música do dia (feramusicas.Pick) é a mesma em todas as máquinas.
//
// Cada música é um documento da coleção (padrão "feramusicas_songs") cujo ID é
// o VideoID do YouTube. Assim o mesmo vídeo não entra duas vezes, nem com dois
// cadastros ao mesmo tempo: o Create do segundo falha com AlreadyExists.
//
// As credenciais vêm do ambiente (normalmente de um .env; veja .env.example):
// FIREBASE_PROJECT_ID e uma conta de serviço, seja pelos campos
// FIREBASE_CLIENT_EMAIL + FIREBASE_PRIVATE_KEY, seja por
// GOOGLE_APPLICATION_CREDENTIALS apontando para o JSON dela.
package firestorecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"fera-games/internal/games/feramusicas"
)

// Variáveis de ambiente lidas por ConfigFromEnv.
const (
	EnvProjectID   = "FIREBASE_PROJECT_ID"
	EnvClientEmail = "FIREBASE_CLIENT_EMAIL"
	EnvPrivateKey  = "FIREBASE_PRIVATE_KEY"
	EnvDatabase    = "FIRESTORE_DATABASE"
	EnvCollection  = "FIRESTORE_COLLECTION"

	// DefaultCollection é a coleção usada quando FIRESTORE_COLLECTION está vazia.
	DefaultCollection = "feramusicas_songs"
)

// ErrNotConfigured indica que FIREBASE_PROJECT_ID não foi definido: o app
// segue com o catálogo em memória.
var ErrNotConfigured = errors.New("firebase não configurado (" + EnvProjectID + " vazio)")

// Config diz onde e com que credenciais falar com o Firestore.
type Config struct {
	ProjectID  string
	Database   string // vazio ou "(default)" = banco padrão
	Collection string
	// ClientEmail e PrivateKey são de uma conta de serviço. Vazios, vale o
	// Application Default Credentials (GOOGLE_APPLICATION_CREDENTIALS ou o
	// login do gcloud), e o emulador (FIRESTORE_EMULATOR_HOST) dispensa ambos.
	ClientEmail string
	PrivateKey  string
}

// ConfigFromEnv lê a configuração de getenv (os.Getenv em produção).
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		ProjectID:   strings.TrimSpace(getenv(EnvProjectID)),
		Database:    strings.TrimSpace(getenv(EnvDatabase)),
		Collection:  strings.TrimSpace(getenv(EnvCollection)),
		ClientEmail: strings.TrimSpace(getenv(EnvClientEmail)),
		// Aceita a chave com quebras de linha reais ou escapadas ("\n"), que é
		// como ela costuma ser colada numa linha só do .env.
		PrivateKey: strings.TrimSpace(strings.ReplaceAll(getenv(EnvPrivateKey), `\n`, "\n")),
	}
	if cfg.ProjectID == "" {
		return Config{}, ErrNotConfigured
	}
	if cfg.Collection == "" {
		cfg.Collection = DefaultCollection
	}
	if strings.Contains(cfg.Collection, "/") {
		return Config{}, fmt.Errorf("%s inválida %q: use só o nome da coleção", EnvCollection, cfg.Collection)
	}
	if (cfg.ClientEmail == "") != (cfg.PrivateKey == "") {
		return Config{}, fmt.Errorf("defina %s e %s juntos (ou nenhum dos dois, para usar GOOGLE_APPLICATION_CREDENTIALS)",
			EnvClientEmail, EnvPrivateKey)
	}
	if cfg.PrivateKey != "" && !strings.Contains(cfg.PrivateKey, "PRIVATE KEY-----") {
		return Config{}, fmt.Errorf("%s não parece uma chave privada (precisa do bloco -----BEGIN PRIVATE KEY-----)", EnvPrivateKey)
	}
	return cfg, nil
}

// credentialsJSON monta o JSON de conta de serviço a partir dos campos do .env.
func (c Config) credentialsJSON() ([]byte, error) {
	return json.Marshal(map[string]string{
		"type":         "service_account",
		"project_id":   c.ProjectID,
		"client_email": c.ClientEmail,
		"private_key":  c.PrivateKey,
		"token_uri":    "https://oauth2.googleapis.com/token",
	})
}

// Catalog implementa feramusicas.Catalog sobre o Firestore. É seguro para uso
// concorrente (o cliente do Firestore é).
type Catalog struct {
	client *firestore.Client
	col    *firestore.CollectionRef
}

var _ feramusicas.Catalog = (*Catalog)(nil)

// New abre o cliente do Firestore. Uma chave privada ilegível falha aqui; o
// resto (conta sem permissão, banco inexistente, sem internet) só aparece na
// primeira operação, porque New não fala com o servidor.
func New(ctx context.Context, cfg Config) (*Catalog, error) {
	var opts []option.ClientOption
	if cfg.ClientEmail != "" {
		creds, err := cfg.credentialsJSON()
		if err != nil {
			return nil, err
		}
		opts = append(opts, option.WithAuthCredentialsJSON(option.ServiceAccount, creds))
	}
	var (
		client *firestore.Client
		err    error
	)
	if cfg.Database == "" || cfg.Database == firestore.DefaultDatabaseID {
		client, err = firestore.NewClient(ctx, cfg.ProjectID, opts...)
	} else {
		client, err = firestore.NewClientWithDatabase(ctx, cfg.ProjectID, cfg.Database, opts...)
	}
	if err != nil {
		return nil, fmt.Errorf("abrir o Firestore: %w", err)
	}
	return &Catalog{client: client, col: client.Collection(cfg.Collection)}, nil
}

// Close fecha a conexão com o Firestore.
func (c *Catalog) Close() error { return c.client.Close() }

// doc é o formato gravado no Firestore.
type doc struct {
	Name        string `firestore:"name"`
	Category    string `firestore:"category"`
	CategoryKey string `firestore:"categoryKey"` // para consultas por categoria no console/futuro
	URL         string `firestore:"url"`
	VideoID     string `firestore:"videoId"`
	// StartSeconds é de onde o trecho começa no vídeo (0 ou ausente = do início).
	StartSeconds float64   `firestore:"startSeconds"`
	CreatedAt    time.Time `firestore:"createdAt,serverTimestamp"`
}

func toDoc(s feramusicas.Song) doc {
	return doc{
		Name: s.Name, Category: s.Category, CategoryKey: feramusicas.CategoryKey(s.Category),
		URL: s.URL, VideoID: s.VideoID, StartSeconds: s.Start.Seconds(),
	}
}

// fromDoc converte um documento em música. Documentos editados à mão no
// console sem os campos obrigatórios são ignorados (ok = false), para não
// quebrar o autocomplete nem o sorteio.
func fromDoc(id string, d doc) (feramusicas.Song, bool) {
	videoID := d.VideoID
	if videoID == "" {
		videoID = id
	}
	s := feramusicas.Song{
		ID: id, Name: feramusicas.CleanText(d.Name), Category: feramusicas.CleanText(d.Category),
		URL: d.URL, VideoID: videoID, Start: startFrom(d.StartSeconds),
	}
	if s.Name == "" || s.Category == "" {
		return feramusicas.Song{}, false
	}
	if _, err := feramusicas.ParseVideoID(feramusicas.WatchURL(videoID)); err != nil {
		return feramusicas.Song{}, false
	}
	return s, true
}

// startFrom converte startSeconds; valores impossíveis (negativo, acima de
// MaxStart, NaN) editados à mão no console valem como 0 em vez de quebrar o
// download da música.
func startFrom(sec float64) time.Duration {
	if !(sec > 0) || sec > feramusicas.MaxStart.Seconds() {
		return 0
	}
	return time.Duration(math.Round(sec*1000)) * time.Millisecond
}

type entry struct {
	song    feramusicas.Song
	created time.Time
}

// List devolve todas as músicas em ordem de cadastro. A ordenação é feita
// aqui, e não com OrderBy, porque o Firestore esconde de um OrderBy os
// documentos sem o campo (ex.: criados à mão no console).
func (c *Catalog) List(ctx context.Context) ([]feramusicas.Song, error) {
	snaps, err := c.col.Documents(ctx).GetAll()
	if err != nil {
		return nil, mapErr(err)
	}
	entries := make([]entry, 0, len(snaps))
	for _, snap := range snaps {
		var d doc
		if snap.DataTo(&d) != nil {
			continue
		}
		if s, ok := fromDoc(snap.Ref.ID, d); ok {
			entries = append(entries, entry{s, d.CreatedAt})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].created.Equal(entries[j].created) {
			return entries[i].created.Before(entries[j].created)
		}
		return entries[i].song.ID < entries[j].song.ID
	})
	out := make([]feramusicas.Song, len(entries))
	for i, e := range entries {
		out[i] = e.song
	}
	return out, nil
}

// ByCategory filtra List em memória: o catálogo é pequeno, e assim valem as
// mesmas regras de comparação (sem caixa, espaços colapsados) do resto do jogo.
func (c *Catalog) ByCategory(ctx context.Context, category string) ([]feramusicas.Song, error) {
	all, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	key := feramusicas.CategoryKey(category)
	var out []feramusicas.Song
	for _, s := range all {
		if feramusicas.CategoryKey(s.Category) == key {
			out = append(out, s)
		}
	}
	return out, nil
}

// Categories são as categorias distintas, com a grafia da música mais antiga.
func (c *Catalog) Categories(ctx context.Context) ([]string, error) {
	all, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	return feramusicas.DistinctCategories(all), nil
}

// Add valida e grava a música com o VideoID como ID do documento.
func (c *Catalog) Add(ctx context.Context, s feramusicas.Song) (feramusicas.Song, error) {
	s, err := feramusicas.PrepareSong(s)
	if err != nil {
		return feramusicas.Song{}, err
	}
	if _, err := c.col.Doc(s.VideoID).Create(ctx, toDoc(s)); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return feramusicas.Song{}, feramusicas.ErrDuplicate
		}
		return feramusicas.Song{}, mapErr(err)
	}
	s.ID = s.VideoID
	return s, nil
}

// Update troca os dados da música id numa transação. Com o mesmo link, só os
// campos mudam (createdAt fica, e a ordem de cadastro também). Com outro link,
// o ID do documento (o VideoID) muda: a transação cria o documento novo, com o
// createdAt do antigo, e apaga o antigo, falhando com ErrDuplicate se o vídeo
// novo já for de outra música.
func (c *Catalog) Update(ctx context.Context, id string, s feramusicas.Song) (feramusicas.Song, error) {
	if id == "" || strings.Contains(id, "/") {
		return feramusicas.Song{}, feramusicas.ErrNotFound
	}
	s, err := feramusicas.PrepareSong(s)
	if err != nil {
		return feramusicas.Song{}, err
	}
	oldRef, newRef := c.col.Doc(id), c.col.Doc(s.VideoID)
	err = c.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		oldSnap, err := tx.Get(oldRef)
		if status.Code(err) == codes.NotFound {
			return feramusicas.ErrNotFound
		}
		if err != nil {
			return err
		}
		if newRef.ID == oldRef.ID {
			d := toDoc(s)
			return tx.Update(oldRef, []firestore.Update{
				{Path: "name", Value: d.Name}, {Path: "category", Value: d.Category},
				{Path: "categoryKey", Value: d.CategoryKey}, {Path: "url", Value: d.URL},
				{Path: "videoId", Value: d.VideoID}, {Path: "startSeconds", Value: d.StartSeconds},
			})
		}
		// Outro vídeo: o documento muda de ID.
		if _, err := tx.Get(newRef); err == nil {
			return feramusicas.ErrDuplicate
		} else if status.Code(err) != codes.NotFound {
			return err
		}
		var old doc
		_ = oldSnap.DataTo(&old)
		d := toDoc(s)
		d.CreatedAt = old.CreatedAt // mantém a ordem de cadastro (zero = hora do servidor)
		if err := tx.Create(newRef, d); err != nil {
			return err
		}
		return tx.Delete(oldRef)
	})
	switch {
	case errors.Is(err, feramusicas.ErrNotFound), errors.Is(err, feramusicas.ErrDuplicate):
		return feramusicas.Song{}, err
	case err != nil:
		return feramusicas.Song{}, mapErr(err)
	}
	s.ID = s.VideoID
	return s, nil
}

// Remove apaga a música; ErrNotFound se ela não existe (mais).
func (c *Catalog) Remove(ctx context.Context, id string) error {
	if id == "" || strings.Contains(id, "/") {
		return feramusicas.ErrNotFound
	}
	if _, err := c.col.Doc(id).Delete(ctx, firestore.Exists); err != nil {
		if status.Code(err) == codes.NotFound {
			return feramusicas.ErrNotFound
		}
		return mapErr(err)
	}
	return nil
}

// mapErr troca códigos do Firestore por frases que dizem o que conferir.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}
	switch status.Code(err) {
	case codes.Unauthenticated:
		return fmt.Errorf("credenciais do Firebase recusadas; confira %s e %s no .env: %w", EnvClientEmail, EnvPrivateKey, err)
	case codes.PermissionDenied:
		return fmt.Errorf("a conta de serviço não tem acesso ao Firestore deste projeto: %w", err)
	case codes.NotFound:
		return fmt.Errorf("banco do Firestore não encontrado; crie o banco no console ou confira %s/%s: %w",
			EnvProjectID, EnvDatabase, err)
	case codes.Unavailable:
		return fmt.Errorf("sem conexão com o Firestore; confira a internet: %w", err)
	}
	return fmt.Errorf("firestore: %w", err)
}
