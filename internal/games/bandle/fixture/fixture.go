// Package fixture traz respostas mínimas do Bandle, no formato real do
// servidor (ver docs/respostas-api.md), para testes sem rede.
package fixture

// PlanningJSON é o payload decifrado de /v2/planning/2026-09-16.txt.
const PlanningJSON = `[
  {
    "id": 1491,
    "day": "2026-09-16",
    "folder": "202608/Only",
    "song": "Sam Smith - I'm Not the Only One",
    "path": "253ab864e93431dfa181",
    "instruments": [
      "drum",
      "[bass] + [electric]",
      "strings",
      "piano",
      "voice",
      "clue"
    ],
    "youtube": "nCkpzqqog4k",
    "youtubeStart": 228,
    "view": 1800,
    "year": 2014,
    "par": 2,
    "bpm": 82,
    "genre": [
      "Soul"
    ],
    "clue": {
      "de": "Ich bin nicht der Einzige",
      "en": "I Am Not the Sole Individual",
      "es": "No soy la única persona",
      "fr": "Je ne suis pas la seule personne"
    },
    "wiki_en": "I%27m_Not_the_Only_One",
    "wiki_fr": "I%27m_Not_the_Only_One",
    "wiki_es": "I%27m_Not_the_Only_One",
    "spotifyId": "7795WJLVKJoAyVoOtCWqXN",
    "stream": 2600,
    "appleMusicId": "1440814433",
    "frontperson": "Sam Smith",
    "sources": [
      "Sam Smith"
    ],
    "minVersion": "8.6.2"
  }
]`

// CatalogJSON é o formato de /v2/guesslist/songs.txt decifrado (reduzido).
// A resposta do dia (Sam Smith - I'm Not the Only One) não vem nele, como
// no servidor real; MergeAnswer a insere.
const CatalogJSON = `[
  {
    "i": 1,
    "t": "Adele - Hello",
    "s": [
      "Adele"
    ]
  },
  {
    "i": 2,
    "t": "Queen - Bohemian Rhapsody",
    "s": [
      "Queen"
    ]
  },
  {
    "i": 3,
    "t": "Sam Smith - Stay With Me",
    "s": [
      "Sam Smith"
    ]
  },
  {
    "i": 4,
    "t": "Sam Smith - Too Good at Goodbyes",
    "s": [
      "Sam Smith"
    ]
  },
  {
    "i": 5,
    "t": "*NSYNC - Bye Bye Bye",
    "s": [
      "*NSYNC"
    ]
  },
  {
    "i": 6,
    "t": "(G)I-DLE - Nxde",
    "s": [
      "(G)I-DLE"
    ]
  },
  {
    "i": 7,
    "t": "Alex Warren - Ordinary",
    "s": [
      "Alex Warren"
    ]
  },
  {
    "i": 8,
    "t": "Max Richter - On the Nature of Daylight",
    "s": [
      "Max Richter"
    ]
  },
  {
    "i": 9,
    "t": "10cc - I'm Not In Love",
    "s": [
      "10cc"
    ]
  },
  {
    "i": 10,
    "t": "The Beatles - Help!",
    "s": [
      "The Beatles"
    ]
  }
]`
