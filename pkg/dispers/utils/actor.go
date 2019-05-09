package utils

type Actor interface {
  /*Domain()                domain
  PrefixDB()              string
  Part()                  string
  Ping()           error // Used in order to launch a process in an appropriate way (API or CMD) depending on isInStackOfConductor
  */
}

type actor struct{
  //domainActor           domain
  //prefix                string // not sure it's very usefull
  //part                  string
  //isInStackOfConductor  bool
}

// NewNewDataAggregation returns a DataAggregation object with the specified values.
func NewActor(/*domain, prefix string, part string, isInStackOfConductor  bool*/) *actor {

	return &actor{
    /*domainActor: domain,
    prefix: prefix,
    part: part,
    isInStackOfConductor: isInStackOfConductor,*/
	}
}
