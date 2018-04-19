package sharing

import (
	"fmt"

	"github.com/cozy/cozy-stack/pkg/instance"
	"github.com/cozy/cozy-stack/pkg/lock"
	multierror "github.com/hashicorp/go-multierror"
)

// UploadMsg is used for jobs on the share-upload worker.
type UploadMsg struct {
	SharingID string `json:"sharing_id"`
	Errors    int    `json:"errors"`
}

// Upload starts uploading files for this sharing
func (s *Sharing) Upload(inst *instance.Instance, errors int) error {
	mu := lock.ReadWrite(inst.Domain + "/sharings/" + s.SID + "/upload")
	mu.Lock()
	defer mu.Unlock()

	var errm error
	var members []*Member
	if !s.Owner {
		members = append(members, &s.Members[0])
	} else {
		for i, m := range s.Members {
			if i == 0 {
				continue
			}
			if m.Status == MemberStatusReady {
				members = append(members, &s.Members[i])
			}
		}
	}

	// TODO what if we have more than BatchSize files to upload?
	for i := 0; i < BatchSize; i++ {
		if len(members) == 0 {
			break
		}
		m := members[0]
		members = members[1:]
		more, err := s.UploadTo(inst, m)
		if err != nil {
			errm = multierror.Append(errm, err)
		}
		if more {
			members = append(members, m)
		}
	}

	if errm != nil {
		s.retryWorker(inst, "share-upload", errors)
		fmt.Printf("DEBUG errm=%s\n", errm)
	}
	return errm
}

// InitialUpload uploads files to just a member, for the first time
func (s *Sharing) InitialUpload(inst *instance.Instance, m *Member) error {
	mu := lock.ReadWrite(inst.Domain + "/sharings/" + s.SID + "/upload")
	mu.Lock()
	defer mu.Unlock()

	// TODO what if we have more than BatchSize files to upload?
	for i := 0; i < BatchSize; i++ {
		more, err := s.UploadTo(inst, m)
		if err != nil {
			return err
		}
		if !more {
			return nil
		}
	}

	return nil
}

// UploadTo uploads one file to the given member. It returns false if there
// are no more files to upload to this member currently.
func (s *Sharing) UploadTo(inst *instance.Instance, m *Member) (bool, error) {
	if m.Instance == "" {
		return false, ErrInvalidURL
	}
	creds := s.FindCredentials(m)
	if creds == nil {
		return false, ErrInvalidSharing
	}

	lastSeq, err := s.getLastSeqNumber(inst, m, "upload")
	if err != nil {
		return false, err
	}
	inst.Logger().WithField("nspace", "upload").Debugf("lastSeq = %s", lastSeq)

	seq := lastSeq // TODO WIP

	if seq == lastSeq {
		return false, nil
	}

	return true, s.UpdateLastSequenceNumber(inst, m, "upload", seq)
}
