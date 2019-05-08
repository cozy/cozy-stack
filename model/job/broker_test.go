package job_test

import (
	"testing"
	"time"

	jobs "github.com/cozy/cozy-stack/model/job"
	"github.com/stretchr/testify/assert"
)

func TestGetJobsBeforeDate(t *testing.T) {
	// Create jobs
	utc, err := time.LoadLocation("")
	assert.NoError(t, err)

	date1 := time.Date(2000, time.January, 1, 1, 1, 1, 1, utc)
	job1 := &jobs.Job{
		Domain:     testInstance.Domain,
		Prefix:     testInstance.DBPrefix(),
		WorkerType: "thumbnail",
		TriggerID:  "foobar",
		Manual:     false,
		State:      jobs.Queued,
		QueuedAt:   date1,
	}
	err = job1.Create()
	assert.NoError(t, err)

	date2 := time.Now()
	job2 := &jobs.Job{
		Domain:     testInstance.Domain,
		Prefix:     testInstance.DBPrefix(),
		WorkerType: "thumbnail",
		TriggerID:  "foobar",
		Manual:     false,
		State:      jobs.Queued,
		QueuedAt:   date2,
	}
	err = job2.Create()
	assert.NoError(t, err)

	date3 := time.Date(2100, time.January, 1, 1, 1, 1, 1, utc)
	job3 := &jobs.Job{
		Domain:     testInstance.Domain,
		Prefix:     testInstance.DBPrefix(),
		WorkerType: "thumbnail",
		TriggerID:  "foobar",
		Manual:     false,
		State:      jobs.Queued,
		QueuedAt:   date3,
	}
	err = job3.Create()
	assert.NoError(t, err)

	jobs, err := jobs.GetJobsBeforeDate(testInstance, time.Now())
	assert.NoError(t, err)

	// We should have only 2 jobs :
	// The first has been queued in the past: OK
	// The second has just been queued: OK
	// The third is queued in the future: NOK
	assert.Equal(t, 2, len(jobs))
}

func TestGetLastsJobs(t *testing.T) {
	j, err := jobs.GetLastsJobs(testInstance, "thumbnail")
	assert.NoError(t, err)
	assert.Equal(t, 3, len(j))

	// Add a job
	myJob := &jobs.Job{
		Domain:     testInstance.Domain,
		Prefix:     testInstance.DBPrefix(),
		WorkerType: "thumbnail",
		TriggerID:  "foobar",
		Manual:     false,
		State:      jobs.Running,
		QueuedAt:   time.Now(),
	}
	err = myJob.Create()
	assert.NoError(t, err)
	j, err = jobs.GetLastsJobs(testInstance, "thumbnail")
	assert.NoError(t, err)
	assert.Equal(t, 4, len(j))

	// Add a job in another queue
	myJob = &jobs.Job{
		Domain:     testInstance.Domain,
		Prefix:     testInstance.DBPrefix(),
		WorkerType: "konnector",
		TriggerID:  "foobar",
		Manual:     false,
		State:      jobs.Errored,
		QueuedAt:   time.Now(),
	}
	err = myJob.Create()
	assert.NoError(t, err)
	j, err = jobs.GetLastsJobs(testInstance, "thumbnail")
	assert.NoError(t, err)
	assert.Equal(t, 4, len(j))
	j, err = jobs.GetLastsJobs(testInstance, "konnector")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(j))

	// No jobs
	j, err = jobs.GetLastsJobs(testInstance, "foobar")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(j))

	// Add a job in the future and assert it is the first one returned
	utc, err := time.LoadLocation("")
	assert.NoError(t, err)
	futureDate := time.Date(2200, time.January, 1, 1, 1, 1, 1, utc)
	myJob = &jobs.Job{
		Domain:     testInstance.Domain,
		Prefix:     testInstance.DBPrefix(),
		WorkerType: "thumbnail",
		TriggerID:  "foobar",
		Manual:     false,
		State:      jobs.Errored,
		QueuedAt:   futureDate,
	}
	err = myJob.Create()
	assert.NoError(t, err)

	j, err = jobs.GetLastsJobs(testInstance, "thumbnail")
	assert.NoError(t, err)

	// One running, one errored, three queued
	assert.Equal(t, 5, len(j))
	assert.Equal(t, futureDate.String(), j[len(j)-1].QueuedAt.String())
	assert.Equal(t, jobs.Errored, j[len(j)-1].State)

}
