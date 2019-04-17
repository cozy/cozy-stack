package dispers

type Dispers struct {
  algoDispers string
  algoML string
  message string
}

const Message string = "Coucou, je suis dispers"

func NewDispers() Dispers {
  d:= Dispers{algoDispers: "simple", algoML: "naivebayes"}
  return d
}

func (d *Dispers) SayHello() string {
   return "Hello World ! I'm the querier. I'm going to launch a ML training"
}
