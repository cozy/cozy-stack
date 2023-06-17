package job_test

import (
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/cozy/cozy-stack/pkg/config/config"
	"github.com/cozy/cozy-stack/tests/testutils"
	"github.com/stretchr/testify/assert"
)

func TestBrokerImplems(t *testing.T) {
	assert.Implements(t, (*job.Broker)(nil), new(job.BrokerMock))
}

func TestBroker(t *testing.T) {
	if testing.Short() {
		t.Skip("an instance is required for this test: test skipped due to the use of --short flag")
	}

	config.UseTestFile(t)
	setup := testutils.NewSetup(t, t.Name())
	testInstance := setup.GetTestInstance()

	t.Run("GetJobsBeforeDate", func(t *testing.T) {
		// Create jobs
		utc, err := time.LoadLocation("")
		assert.NoError(t, err)

		date1 := time.Date(2000, time.January, 1, 1, 1, 1, 1, utc)
		job1 := &job.Job{
			Domain:     testInstance.Domain,
			Prefix:     testInstance.DBPrefix(),
			WorkerType: "thumbnail",
			TriggerID:  "foobar",
			Manual:     false,
			State:      job.Queued,
			QueuedAt:   date1,
		}
		err = job1.Create()
		assert.NoError(t, err)

		date2 := time.Now()
		job2 := &job.Job{
			Domain:     testInstance.Domain,
			Prefix:     testInstance.DBPrefix(),
			WorkerType: "thumbnail",
			TriggerID:  "foobar",
			Manual:     false,
			State:      job.Queued,
			QueuedAt:   date2,
		}
		err = job2.Create()
		assert.NoError(t, err)

		date3 := time.Date(2100, time.January, 1, 1, 1, 1, 1, utc)
		job3 := &job.Job{
			Domain:     testInstance.Domain,
			Prefix:     testInstance.DBPrefix(),
			WorkerType: "thumbnail",
			TriggerID:  "foobar",
			Manual:     false,
			State:      job.Queued,
			QueuedAt:   date3,
		}
		err = job3.Create()
		assert.NoError(t, err)

		allJobs, err := job.GetAllJobs(testInstance)
		assert.NoError(t, err)
		assert.Equal(t, 3, len(allJobs))

		jobs := job.FilterJobsBeforeDate(allJobs, time.Now())

		// We should have only 2 jobs :
		// The first has been queued in the past: OK
		// The second has just been queued: OK
		// The third is queued in the future: NOK
		assert.Equal(t, 2, len(jobs))
	})

	t.Run("GetLastsJobs", func(t *testing.T) {
		allJobs, err := job.GetAllJobs(testInstance)
		assert.NoError(t, err)
		j, err := job.GetLastsJobs(allJobs, "thumbnail")
		assert.NoError(t, err)
		assert.Equal(t, 3, len(j))

		// Add a job
		myJob := &job.Job{
			Domain:     testInstance.Domain,
			Prefix:     testInstance.DBPrefix(),
			WorkerType: "thumbnail",
			TriggerID:  "foobar",
			Manual:     false,
			State:      job.Running,
			QueuedAt:   time.Now(),
		}
		err = myJob.Create()
		assert.NoError(t, err)
		allJobs, err = job.GetAllJobs(testInstance)
		assert.NoError(t, err)
		j, err = job.GetLastsJobs(allJobs, "thumbnail")
		assert.NoError(t, err)
		assert.Equal(t, 4, len(j))

		// Add a job in another queue
		myJob = &job.Job{
			Domain:     testInstance.Domain,
			Prefix:     testInstance.DBPrefix(),
			WorkerType: "konnector",
			TriggerID:  "foobar",
			Manual:     false,
			State:      job.Errored,
			QueuedAt:   time.Now(),
		}
		err = myJob.Create()
		assert.NoError(t, err)
		allJobs, err = job.GetAllJobs(testInstance)
		assert.NoError(t, err)
		j, err = job.GetLastsJobs(allJobs, "thumbnail")
		assert.NoError(t, err)
		assert.Equal(t, 4, len(j))
		allJobs, err = job.GetAllJobs(testInstance)
		assert.NoError(t, err)
		j, err = job.GetLastsJobs(allJobs, "konnector")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(j))

		// No jobs
		allJobs, err = job.GetAllJobs(testInstance)
		assert.NoError(t, err)
		j, err = job.GetLastsJobs(allJobs, "foobar")
		assert.NoError(t, err)
		assert.Equal(t, 0, len(j))

		// Add a job in the future and assert it is the first one returned
		utc, err := time.LoadLocation("")
		assert.NoError(t, err)
		futureDate := time.Date(2200, time.January, 1, 1, 1, 1, 1, utc)
		myJob = &job.Job{
			Domain:     testInstance.Domain,
			Prefix:     testInstance.DBPrefix(),
			WorkerType: "thumbnail",
			TriggerID:  "foobar",
			Manual:     false,
			State:      job.Errored,
			QueuedAt:   futureDate,
		}
		err = myJob.Create()
		assert.NoError(t, err)

		allJobs, err = job.GetAllJobs(testInstance)
		assert.NoError(t, err)
		j, err = job.GetLastsJobs(allJobs, "thumbnail")
		assert.NoError(t, err)

		// One running, one errored, three queued
		assert.Equal(t, 5, len(j))
		assert.Equal(t, futureDate.String(), j[len(j)-1].QueuedAt.String())
		assert.Equal(t, job.Errored, j[len(j)-1].State)
	})
}
