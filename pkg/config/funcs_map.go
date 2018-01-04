package config

import (
	"math"
	"reflect"
	"strconv"
)

var numericFuncsMap = map[string]interface{}{
	"add": func(i ...interface{}) int64 {
		var a int64 = 0
		for _, b := range i {
			a += toInt64(b)
		}
		return a
	},
	"sub": func(a, b interface{}) int64 { return toInt64(a) - toInt64(b) },
	"div": func(a, b interface{}) int64 { return toInt64(a) / toInt64(b) },
	"mod": func(a, b interface{}) int64 { return toInt64(a) % toInt64(b) },
	"mul": func(a interface{}, v ...interface{}) int64 {
		val := toInt64(a)
		for _, b := range v {
			val = val * toInt64(b)
		}
		return val
	},
}

// toInt64 converts integer types to 64-bit integers
func toInt64(v interface{}) int64 {
	if str, ok := v.(string); ok {
		iv, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return 0
		}
		return iv
	}

	val := reflect.Indirect(reflect.ValueOf(v))
	switch val.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int:
		return val.Int()
	case reflect.Uint8, reflect.Uint16, reflect.Uint32:
		return int64(val.Uint())
	case reflect.Uint, reflect.Uint64:
		tv := val.Uint()
		if tv <= math.MaxInt64 {
			return int64(tv)
		}
		// TODO: What is the sensible thing to do here?
		return math.MaxInt64
	case reflect.Float32, reflect.Float64:
		return int64(val.Float())
	case reflect.Bool:
		if val.Bool() {
			return 1
		}
		return 0
	default:
		return 0
	}
}
