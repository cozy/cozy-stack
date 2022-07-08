package job_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/robfig/cron/v3"
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
	spec, err = p.Parse(job.DailyKind, "before 5am")
	require.NoError(t, err)
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 5)

	spec, err = p.Parse(job.DailyKind, "after 10pm")
	require.NoError(t, err)
	assert.Equal(t, spec.AfterHour, 22)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse(job.DailyKind, "between 8am and 6pm")
	require.NoError(t, err)
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
	_, err = p.Parse(job.DailyKind, "on monday")
	assert.Error(t, err)
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

	// Not yet supported, but we may want it in the future
	_, err = p.Parse(job.DailyKind, "on the 1-5")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "between 9pm and 3am")
	assert.Error(t, err)
	_, err = p.Parse(job.WeeklyKind, "on the 1-15")
	assert.Error(t, err)
	_, err = p.Parse(job.MonthlyKind, "on monday")
	assert.Error(t, err)
}

func TestToRandomCrontab(t *testing.T) {
	day := 24 * time.Hour
	cronParser := cron.NewParser(cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	seed := fmt.Sprintf("%d", time.Now().UnixNano())

	// Monthly
	spec := job.PeriodicSpec{
		Frequency:   job.MonthlyKind,
		DaysOfMonth: []int{3, 4, 5, 6, 7},
		AfterHour:   8,
		BeforeHour:  16,
	}
	crontab := spec.ToRandomCrontab(seed)
	fields := strings.Fields(crontab)
	require.Len(t, fields, 6)

	second, err := strconv.Atoi(fields[0])
	assert.NoError(t, err)
	assert.True(t, 0 <= second && second < 60)

	minute, err := strconv.Atoi(fields[1])
	assert.NoError(t, err)
	assert.True(t, 0 <= minute && minute < 60)

	hour, err := strconv.Atoi(fields[2])
	assert.NoError(t, err)
	assert.True(t, 8 <= hour && hour < 16)

	dom, err := strconv.Atoi(fields[3]) // day of month
	assert.NoError(t, err)
	assert.True(t, 3 <= dom && dom <= 7)

	assert.Equal(t, fields[4], "*") // month
	assert.Equal(t, fields[5], "*") // day of week

	// Check that two successive executions are separated by about 30 days
	schedule, err := cronParser.Parse(crontab)
	require.NoError(t, err)
	exec := schedule.Next(time.Now())
	next := schedule.Next(exec)
	assert.WithinDuration(t, exec.Add(30*day), next, 3*day)

	// Weekly
	spec = job.PeriodicSpec{
		Frequency:  job.WeeklyKind,
		DaysOfWeek: []int{1, 2, 3, 4, 5},
		AfterHour:  0,
		BeforeHour: 5,
	}
	crontab = spec.ToRandomCrontab(seed)
	fields = strings.Fields(crontab)
	require.Len(t, fields, 6)

	second, err = strconv.Atoi(fields[0])
	assert.NoError(t, err)
	assert.True(t, 0 <= second && second < 60)

	minute, err = strconv.Atoi(fields[1])
	assert.NoError(t, err)
	assert.True(t, 0 <= minute && minute < 60)

	hour, err = strconv.Atoi(fields[2])
	assert.NoError(t, err)
	assert.True(t, 0 <= hour && hour < 5)

	assert.Equal(t, fields[3], "*") // day of month
	assert.Equal(t, fields[4], "*") // month

	dow, err := strconv.Atoi(fields[5]) // day of week
	assert.NoError(t, err)
	assert.True(t, 1 <= dow && dow <= 5)

	// Check that two successive executions are separated by about 7 days
	schedule, err = cronParser.Parse(crontab)
	require.NoError(t, err)
	exec = schedule.Next(time.Now())
	next = schedule.Next(exec)
	assert.WithinDuration(t, exec.Add(7*day), next, 3*time.Minute)

	// Daily
	spec = job.PeriodicSpec{
		Frequency:  job.DailyKind,
		AfterHour:  16,
		BeforeHour: 24,
	}
	crontab = spec.ToRandomCrontab(seed)
	fields = strings.Fields(crontab)
	require.Len(t, fields, 6)

	second, err = strconv.Atoi(fields[0])
	assert.NoError(t, err)
	assert.True(t, 0 <= second && second < 60)

	minute, err = strconv.Atoi(fields[1])
	assert.NoError(t, err)
	assert.True(t, 0 <= minute && minute < 60)

	hour, err = strconv.Atoi(fields[2])
	assert.NoError(t, err)
	assert.True(t, 16 <= hour && hour < 24)

	assert.Equal(t, fields[3], "*") // day of month
	assert.Equal(t, fields[4], "*") // month
	assert.Equal(t, fields[5], "*") // day of week

	// Check that two successive executions are separated by about 7 days
	schedule, err = cronParser.Parse(crontab)
	require.NoError(t, err)
	exec = schedule.Next(time.Now())
	next = schedule.Next(exec)
	assert.WithinDuration(t, exec.Add(day), next, 3*time.Minute)
}
