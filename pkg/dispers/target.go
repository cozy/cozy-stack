package enclave

type InputT struct {
  Localquery        string    `json:"localquery,omitempty"`
  ListsOfAdresses   []string  `json:"adresses,omitempty"`
}

type Query struct {
  Query string `json:"query,omitempty"`
  OAuth string `json:"oauth,omitempty"`
}

type OutputT struct {
  Queries []Query `json:"queries,omitempty"`
}

func GetTokens(in InputT) OutputT {
  return OutputT{
    Queries : []Query{
      Query{
        Query : "Abc",
        OAuth : "Oauth1",
      },
      Query{
        Query : "fheu",
        OAuth : "Oauth2",
      },
    },
  }
}
