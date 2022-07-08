package job_test

import (
	"testing"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPeriodicParser(t *testing.T) {
	p := job.NewPeriodicParser()

	spec, err := p.Parse(job.WeeklyKind, "")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse(job.MonthlyKind, "")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfMonth, []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	// Month days
	spec, err = p.Parse(job.MonthlyKind, "on the 1")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfMonth, []int{1})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse(job.MonthlyKind, "on the 1,16")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfMonth, []int{1, 16})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse(job.MonthlyKind, "on the 1-3,5-7,9-11")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfMonth, []int{1, 2, 3, 5, 6, 7, 9, 10, 11})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	// Week days
	spec, err = p.Parse(job.WeeklyKind, "on monday")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{1})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse(job.WeeklyKind, "on mon,wed,fri")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{1, 3, 5})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse(job.WeeklyKind, "on wed-fri")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{3, 4, 5})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse(job.WeeklyKind, "on fri-mon")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{5, 6, 0, 1})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse(job.WeeklyKind, "on weekday")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{1, 2, 3, 4, 5})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse(job.WeeklyKind, "on weekend")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 6})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse(job.WeeklyKind, "on weekday,saturday")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	// Hours
	spec, err = p.Parse(job.WeeklyKind, "before 5am")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 5)

	spec, err = p.Parse(job.WeeklyKind, "after 10pm")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 22)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse(job.WeeklyKind, "between 8am and 6pm")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 8)
	assert.Equal(t, spec.BeforeHour, 18)

	spec, err = p.Parse(job.WeeklyKind, "between 12am and 12pm")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 12)

	spec, err = p.Parse(job.WeeklyKind, "on monday before 9am")
	require.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{1})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 9)

	// Errors
	_, err = p.Parse(job.WeeklyKind, "xyz")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "on")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "on xyz")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "on mon,")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "on mon-")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "on -mon")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "on mon-weekend")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "after")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "after xyz")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "before")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "before xyz")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "between 8am and 8am")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "on the monday")
	assert.Error(t, err)
	_, err = p.Parse(job.MonthlyKind, "on the")
	assert.Error(t, err)
	_, err = p.Parse(job.MonthlyKind, "on the xyz")
	assert.Error(t, err)
	_, err = p.Parse(job.MonthlyKind, "on 1-")
	assert.Error(t, err)
	_, err = p.Parse(job.MonthlyKind, "on 1-5")
	assert.Error(t, err)
	_, err = p.Parse(job.MonthlyKind, "on the 5-1")
	assert.Error(t, err)
	_, err = p.Parse(job.MonthlyKind, "on the 5-5")
	assert.Error(t, err)

	// Not yep supported, but we may want it in the future
	_, err = p.Parse(job.WeeklyKind, "between 9pm and 3am")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "on the 1-15")
	assert.Error(t, err)
	_, err = p.Parse(job.MonthlyKind, "on monday")
	assert.Error(t, err)
}
