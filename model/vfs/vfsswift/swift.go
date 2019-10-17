package vfsswift

import (
	"errors"
	"time"

	multierror "github.com/hashicorp/go-multierror"
	"github.com/ncw/swift"
)

// maxNbFilesToDelete is the maximal number of files that we will try to delete
// in a single call to swift.
const maxNbFilesToDelete = 8000

// maxSimultaneousCalls is the maximal number of simultaneous calls to Swift to
// delete files in the same container.
const maxSimultaneousCalls = 8

var errFailFast = errors.New("fail fast")

// DeleteContainer removes all the files inside the given container, and then
// deletes it.
func DeleteContainer(c *swift.Connection, container string) error {
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

	// XXX Swift has told us that all the files have been deleted on the bulk
	// delete, but it only means that they have been deleted on one object
	// server (at least). And, when we try to delete the container, Swift can
	// send an error as some container servers will still have objects
	// registered for this container. We will try several times to delete the
	// container to work-around this limitation.
	for i := 0; i < 5; i++ {
		err = c.ContainerDelete(container)
		if err == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return err
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
		if end > len(objectNames) {
			end = len(objectNames)
		}
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
