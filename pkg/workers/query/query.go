package push

import (
	"crypto/md5"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cozy/cozy-stack/pkg/config"
	"github.com/cozy/cozy-stack/pkg/jobs"
	"github.com/cozy/cozy-stack/pkg/oauth"
	"github.com/cozy/cozy-stack/pkg/dispers"

)

var (

)

func init() {
	jobs.AddWorker(&jobs.WorkerConfig{
		WorkerType:   "query",
		Concurrency:  runtime.NumCPU(),
		// WorkerInit:   Init,
		WorkerFunc:   Worker,
	})
}

// Algorith contains the configuration of a ML algorithm.
type Training struct {
	TrainingID string `json:"training_id"`
	Source         string `json:"source"`
	Collapsible    bool   `json:"collapsible,omitempty"`

	Data map[string]interface{} `json:"data,omitempty"`
}

// Init initializes the necessary global clients
func Init() (err error) {
  somethingGood = true
	if somethingGood {
    errOfSomeTask = nil
		if errOfSomeTask != nil {
			return errOfSomeTask
		}
	}
	return
}

// Worker is the worker that just logs its message (useful for debugging)
func Worker(ctx *jobs.WorkerContext) error {
  var trn Training

  // Get Algorithm from Doctypes

  err = query(ctx, c, &trn)

  // if error, print logs
	if err != nil {
		ctx.Logger().
			WithFields(logrus.Fields{
				"algo":       trn.algo(),
        "dataset":    trn.dataset(),
        "formula":    trn.formula()
			}).
			Warnf("could not train algorithm : %s", err)
	}
	return nil
}

func query(ctx *jobs.WorkerContext, c *oauth.Client, trn *Training) error {
	switch trn.DispersAlgo {
	case simple:
		return NewDispersSimple(trn)
	case multiple:
		return NewDispersMultiple(trn)
  case sequential:
    return NewDispersSequential(trn)
  case stepbystep:
    return NewDispersStepbyStep(trn)
	default:
		return fmt.Errorf(training: unknown dispers' algorithm %q", trn.DispersAlgo)
	}
}


func hashSource(source string) []byte {
	h := md5.New()
	h.Write([]byte(source))
	return h.Sum(nil)
}
