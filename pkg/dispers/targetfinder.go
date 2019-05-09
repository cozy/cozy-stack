package dispers


type Adresses struct {
  ListOfAdresses []string `json:"adresses,omitempty"`
}

type InputTF struct {
  ListOfConcepts []Adresses `json:"concepts,omitempty"`
}

func SelectAdresses( in InputTF) []string {
  return []string{ "45rbgbee6", "4ffef", "7e8f7r5r", "frt47rr7c8", "7c846cf7es", "fs85fe7z8s"}
}
