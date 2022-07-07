package job_test

import (
	"testing"

	"github.com/cozy/cozy-stack/model/job"
	"github.com/stretchr/testify/assert"
)

func TestPeriodicParser(t *testing.T) {
	p := job.NewPeriodicParser()

	spec, err := p.Parse("")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse("on monday")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{1})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse("on mon,wed,fri")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{1, 3, 5})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse("on wed-fri")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{3, 4, 5})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse("on fri-mon")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{5, 6, 0, 1})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse("on weekday")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{1, 2, 3, 4, 5})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse("on weekend")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 6})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse("on weekday,saturday")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse("before 5am")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 5)

	spec, err = p.Parse("after 10pm")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 22)
	assert.Equal(t, spec.BeforeHour, 24)

	spec, err = p.Parse("between 8am and 6pm")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 8)
	assert.Equal(t, spec.BeforeHour, 18)

	spec, err = p.Parse("between 12am and 12pm")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{0, 1, 2, 3, 4, 5, 6})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 12)

	spec, err = p.Parse("on monday before 9am")
	assert.NoError(t, err)
	assert.Equal(t, spec.DaysOfWeek, []int{1})
	assert.Equal(t, spec.AfterHour, 0)
	assert.Equal(t, spec.BeforeHour, 9)

	_, err = p.Parse("xyz")
	assert.Error(t, err)
	_, err = p.Parse("on")
	assert.Error(t, err)
	_, err = p.Parse("on xyz")
	assert.Error(t, err)
	_, err = p.Parse("on mon,")
	assert.Error(t, err)
	_, err = p.Parse("on mon-")
	assert.Error(t, err)
	_, err = p.Parse("on -mon")
	assert.Error(t, err)
	_, err = p.Parse("on mon-weekend")
	assert.Error(t, err)
	_, err = p.Parse("after")
	assert.Error(t, err)
	_, err = p.Parse("after xyz")
	assert.Error(t, err)
	_, err = p.Parse("before")
	assert.Error(t, err)
	_, err = p.Parse("before xyz")
	assert.Error(t, err)
	_, err = p.Parse("between 8am and 8am")
	assert.Error(t, err)
	_, err = p.Parse("between 9pm and 3am")
	assert.Error(t, err)
}
