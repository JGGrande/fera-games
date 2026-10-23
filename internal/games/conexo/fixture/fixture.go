// Package fixture traz respostas mínimas do Conexo, no formato real do
// Firestore REST (ver docs/conexo-protocol.md §3), para testes sem rede.
package fixture

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// Doc1117JSON é GET …/documents/conexo-daily-pt/1117 (2026-10-22), sem os
// campos opcionais (corrections, version, images).
const Doc1117JSON = `{
  "name": "projects/daydashgames/databases/(default)/documents/conexo-daily-pt/1117",
  "fields": {
    "id": {"integerValue": "1117"},
    "groups": {"arrayValue": {"values": [
      {"mapValue": {"fields": {"number": {"integerValue": "1"}, "theme": {"stringValue": "Equipamentos de pesca"},
        "words": {"arrayValue": {"values": [{"stringValue": "molinete"}, {"stringValue": "vara"}, {"stringValue": "linha"}, {"stringValue": "anzol"}]}}}}},
      {"mapValue": {"fields": {"number": {"integerValue": "2"}, "theme": {"stringValue": "Moldável"},
        "words": {"arrayValue": {"values": [{"stringValue": "maleável"}, {"stringValue": "elástico"}, {"stringValue": "mole"}, {"stringValue": "flexível"}]}}}}},
      {"mapValue": {"fields": {"number": {"integerValue": "3"}, "theme": {"stringValue": "Componentes de uma armadura medieval"},
        "words": {"arrayValue": {"values": [{"stringValue": "manopla"}, {"stringValue": "espaldar"}, {"stringValue": "braçal"}, {"stringValue": "greva"}]}}}}},
      {"mapValue": {"fields": {"number": {"integerValue": "4"}, "theme": {"stringValue": "Truco"},
        "words": {"arrayValue": {"values": [{"stringValue": "manilha"}, {"stringValue": "zap"}, {"stringValue": "vira"}, {"stringValue": "vaza"}]}}}}}
    ]}},
    "startingBoard": {"arrayValue": {"values": [
      {"stringValue": "manilha"}, {"stringValue": "espaldar"}, {"stringValue": "vara"}, {"stringValue": "braçal"},
      {"stringValue": "flexível"}, {"stringValue": "linha"}, {"stringValue": "maleável"}, {"stringValue": "vira"},
      {"stringValue": "greva"}, {"stringValue": "elástico"}, {"stringValue": "manopla"}, {"stringValue": "mole"},
      {"stringValue": "molinete"}, {"stringValue": "vaza"}, {"stringValue": "anzol"}, {"stringValue": "zap"}
    ]}},
    "credits": {"mapValue": {"fields": {"author": {"stringValue": "Anna Duzzi"}, "reviewer": {"stringValue": "Elysa Marques"}}}}
  },
  "createTime": "2026-09-01T17:42:42.500266Z",
  "updateTime": "2026-09-01T17:42:42.500266Z"
}`

// Doc1117Corrected é o #1117 com version 2 e correções de exibição (uma
// palavra e um tema), como os 37 documentos pt que têm corrections.
var Doc1117Corrected = WithFields(`"version": {"integerValue": "2"},
    "corrections": {"mapValue": {"fields": {"vaza": {"stringValue": "vasa"}, "Truco": {"stringValue": "Truco!"}}}}`)

// WithFields devolve o #1117 com campos extras em "fields" (JSON tipado).
func WithFields(extra string) string {
	const anchor = `"credits": {`
	return strings.Replace(Doc1117JSON, anchor, extra+",\n    "+anchor, 1)
}

// Corpos de erro do Firestore REST.
const (
	ForbiddenJSON = `{"error": {"code": 403, "message": "Missing or insufficient permissions.", "status": "PERMISSION_DENIED"}}`
	NotFoundJSON  = `{"error": {"code": 404, "message": "Document \"projects/daydashgames/databases/(default)/documents/conexo-daily-pt/9999\" not found.", "status": "NOT_FOUND"}}`
)

// Token monta um JWT de App Check com o exp dado. A assinatura é lixo: o
// cliente só lê o exp, quem valida é o servidor.
func Token(exp time.Time) string {
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"kid":"A71KHw","typ":"JWT","alg":"RS256"}`))
	payload := enc.EncodeToString(fmt.Appendf(nil,
		`{"sub":"1:689008611670:web:45c4d4dbce26e56deb46f7","provider":"recaptcha_enterprise","iat":%d,"exp":%d}`,
		exp.Add(-168*time.Hour).Unix(), exp.Unix()))
	return header + "." + payload + ".c2lnbmF0dXJh"
}
