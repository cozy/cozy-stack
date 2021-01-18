package config

import "fmt"

// Normalize can be used on a config loaded from a Yaml file so that it can
// encoded as JSON. Go doesn't want to encode map[interface{}]interface{} to
// JSON, so we need to do some tricks to have a type that is accepted by Go.
func Normalize(input map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(input))
	for k, v := range input {
		switch v := v.(type) {
		case map[interface{}]interface{}:
			normalized[k] = doNormalizeMap(v)
		case []interface{}:
			normalized[k] = doNormalizeSlice(v)
		default:
			normalized[k] = v
		}
	}
	return normalized
}

func doNormalizeMap(input map[interface{}]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(input))
	for k, v := range input {
		key := fmt.Sprintf("%v", k)
		switch v := v.(type) {
		case map[interface{}]interface{}:
			normalized[key] = doNormalizeMap(v)
		case []interface{}:
			normalized[key] = doNormalizeSlice(v)
		default:
			normalized[key] = v
		}
	}
	return normalized
}

func doNormalizeSlice(input []interface{}) []interface{} {
	normalized := make([]interface{}, len(input))
	for i, v := range input {
		switch v := v.(type) {
		case map[interface{}]interface{}:
			normalized[i] = doNormalizeMap(v)
		case []interface{}:
			normalized[i] = doNormalizeSlice(v)
		default:
			normalized[i] = v
		}
	}
	return normalized
}
