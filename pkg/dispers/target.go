package enclave

import "github.com/cozy/cozy-stack/pkg/dispers/dispers"

func GetTokens(in dispers.InputT) dispers.OutputT {
	return dispers.OutputT{
		Queries: []dispers.Query{
			dispers.Query{
				Query: "Abc",
				OAuth: "Oauth1",
			},
			dispers.Query{
				Query: "fheu",
				OAuth: "Oauth2",
			},
		},
	}
}
