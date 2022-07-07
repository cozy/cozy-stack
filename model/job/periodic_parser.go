package job

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// PeriodicParser can be used to parse @weekly and @monthly trigger arguments.
// It can parse a string like "on monday between 8am and 6pm".
type PeriodicParser struct{}

// PeriodicSpec is the result of a successful parsing
type PeriodicSpec struct {
	DaysOfWeek []int // a slice of acceptable days, from 0 for sunday to 6 for saturday
	AfterHour  int   // an hour between 0 and 23
	BeforeHour int   // an hour between 1 and 24
}

// NewPeriodicParser creates a PeriodicParser.
func NewPeriodicParser() PeriodicParser {
	return PeriodicParser{}
}

// Parse will transform a string like "on monday" to a PeriodicSpec, or will
// return an error if the format is not supported.
func (p *PeriodicParser) Parse(periodic string) (*PeriodicSpec, error) {
	fields := strings.Fields(periodic)
	spec := PeriodicSpec{
		DaysOfWeek: []int{0, 1, 2, 3, 4, 5, 6},
		AfterHour:  0,
		BeforeHour: 24,
	}

	for len(fields) > 0 {
		switch fields[0] {
		case "on":
			if len(fields) == 1 {
				return nil, errors.New("expecting a day after 'on' keyword")
			}
			dow, err := p.parseDaysOfWeek(fields[1])
			if err != nil {
				return nil, err
			}
			spec.DaysOfWeek = dow
			fields = fields[2:]
		case "before", "and":
			if len(fields) == 1 {
				return nil, fmt.Errorf("expecting an hour after '%s' keyword", fields[0])
			}
			hour, err := p.parseHour(fields[1])
			if err != nil {
				return nil, err
			}
			if hour == 0 {
				hour = 24
			}
			spec.BeforeHour = hour
			fields = fields[2:]
		case "after", "between":
			if len(fields) == 1 {
				return nil, fmt.Errorf("expecting an hour after '%s' keyword", fields[0])
			}
			hour, err := p.parseHour(fields[1])
			if err != nil {
				return nil, err
			}
			spec.AfterHour = hour
			fields = fields[2:]
		default:
			return nil, fmt.Errorf("invalid field %q", fields[0])
		}
	}

	if spec.AfterHour >= spec.BeforeHour {
		return nil, errors.New("invalid hours range")
	}

	return &spec, nil
}

func (p *PeriodicParser) parseDaysOfWeek(field string) ([]int, error) {
	var days []int
	parts := strings.Split(field, ",")
	for _, part := range parts {
		if strings.Contains(part, "-") {
			splitted := strings.SplitN(part, "-", 2)
			from, err := p.parseDayOfWeek(splitted[0])
			if err != nil {
				return nil, err
			}
			to, err := p.parseDayOfWeek(splitted[1])
			if err != nil {
				return nil, err
			}
			if from == to {
				return nil, errors.New("invalid range")
			} else if from > to {
				to += 7
			}
			for i := from; i <= to; i++ {
				days = append(days, i%7)
			}
		} else if part == "weekday" {
			days = append(days, 1, 2, 3, 4, 5)
		} else if part == "weekend" {
			days = append(days, 0, 6)
		} else {
			dow, err := p.parseDayOfWeek(part)
			if err != nil {
				return nil, err
			}
			days = append(days, dow)
		}
	}
	return days, nil
}

func (p *PeriodicParser) parseDayOfWeek(day string) (int, error) {
	switch day {
	case "sun", "sunday":
		return 0, nil
	case "mon", "monday":
		return 1, nil
	case "tue", "tuesday":
		return 2, nil
	case "wed", "wednesday":
		return 3, nil
	case "thu", "thursday":
		return 4, nil
	case "fri", "friday":
		return 5, nil
	case "sat", "saturday":
		return 6, nil
	default:
		return -1, fmt.Errorf("cannot parse %q as a day", day)
	}
}

func (p *PeriodicParser) parseHour(hour string) (int, error) {
	if strings.HasSuffix(hour, "am") {
		h, err := strconv.Atoi(strings.TrimSuffix(hour, "am"))
		if err != nil {
			return -1, err
		}
		if h <= 0 || h > 12 {
			return -1, errors.New("invalid hour")
		}
		if h == 12 {
			return 0, nil
		}
		return h, nil
	}

	if strings.HasSuffix(hour, "pm") {
		h, err := strconv.Atoi(strings.TrimSuffix(hour, "pm"))
		if err != nil {
			return -1, err
		}
		if h <= 0 || h > 12 {
			return -1, errors.New("invalid hour")
		}
		if h == 12 {
			return 12, nil
		}
		return h + 12, nil
	}

	return -1, errors.New("invalid hour")
}
