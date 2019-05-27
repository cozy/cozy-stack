package enclave

import (
	"github.com/cozy/cozy-stack/pkg/dispers/dispers"
)

// SelectAddresses apply the target profile over lists of addresses
func SelectAddresses(in dispers.InputTF) ([]string, error) {

	finalList, err := in.TargetProfile.Compute(in.ListsOfAddresses)
	// TODO: Encrypt final list
	return finalList, err
}
