package vfsswift

import (
	"github.com/cozy/swift"
	multierror "github.com/hashicorp/go-multierror"
)

// maxNbFilesToDelete is the maximal number of files that we will try to delete
// in a single call to swift.
const maxNbFilesToDelete = 8000

// maxSimultaneousCalls is the maximal number of simultaneous calls to Swift to
// delete files in the same container.
const maxSimultaneousCalls = 8

func deleteContainer(c *swift.Connection, container string) error {
	_, _, err := c.Container(container)
	if err == swift.ContainerNotFound {
		return nil
	}
	if err != nil {
		return err
	}
	objectNames, err := c.ObjectNamesAll(container, nil)
	if err != nil {
		return err
	}
	if len(objectNames) > 0 {
		if err = deleteContainerFiles(c, container, objectNames); err != nil {
			return err
		}
	}
	return c.ContainerDelete(container)
}

func deleteContainerFiles(c *swift.Connection, container string, objectNames []string) error {
	nb := 1 + (len(objectNames)-1)/maxNbFilesToDelete
	ch := make(chan error)

	// Use a system of tokens to limit the number of simultaneous calls to
	// Swift: only a goroutine that has a token can make a call.
	tokens := make(chan int, maxSimultaneousCalls)
	for k := 0; k < maxSimultaneousCalls; k++ {
		tokens <- k
	}

	for i := 0; i < nb; i++ {
		begin := i * maxNbFilesToDelete
		end := (i + 1) * maxNbFilesToDelete
		objectToDelete := objectNames[begin:end]
		go func() {
			k := <-tokens
			_, err := c.BulkDelete(container, objectToDelete)
			ch <- err
			tokens <- k
		}()
	}

	var errm error
	for i := 0; i < nb; i++ {
		if err := <-ch; err != nil {
			errm = multierror.Append(errm, err)
		}
	}
	// Get back the tokens to ensure that each goroutine can finish.
	for k := 0; k < maxSimultaneousCalls; k++ {
		<-tokens
	}
	return errm
}
