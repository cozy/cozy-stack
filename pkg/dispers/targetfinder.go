package enclave

import (
	"github.com/cozy/cozy-stack/pkg/dispers/dispers"
)

func SelectAddresses(in dispers.InputTF) ([]string, error) {

	finalList, err := in.TargetProfile.Compute(in.ListsOfAddresses)
	// TODO: Encrypt final list
	return finalList, err
}
