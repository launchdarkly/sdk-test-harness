package data

import (
	"encoding/json"
	"fmt"

	yaml "gopkg.in/yaml.v3"
)

// ParseJSONOrYAML is used in the same way as json.Unmarshal, but if the data is YAML and not
// JSON, it will convert the YAML to JSON and then parse it as JSON.
func ParseJSONOrYAML(data []byte, target interface{}) error {
	if err := json.Unmarshal(data, target); err == nil {
		return nil
	}
	var rawStructure interface{}
	if err := yaml.Unmarshal(data, &rawStructure); err != nil {
		return err
	}
	normalized, err := normalizeParsedYAMLForJSON(rawStructure)
	if err != nil {
		return err
	}
	jsonData, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonData, target)
}

func normalizeParsedYAMLForJSON(data interface{}) (interface{}, error) {
	switch data := data.(type) {
	case []interface{}:
		arrayOut := make([]interface{}, 0)
		for _, v := range data {
			v1, err := normalizeParsedYAMLForJSON(v)
			if err != nil {
				return nil, err
			}
			arrayOut = append(arrayOut, v1)
		}
		return arrayOut, nil
	case map[string]interface{}:
		mapOut := make(map[string]interface{})
		for k, v := range data {
			v1, err := normalizeParsedYAMLForJSON(v)
			if err != nil {
				return nil, err
			}
			mapOut[k] = v1
		}
		return mapOut, nil
	case map[interface{}]interface{}:
		mapOut := make(map[string]interface{})
		for k, v := range data {
			switch key := k.(type) {
			case string:
				v1, err := normalizeParsedYAMLForJSON(v)
				if err != nil {
					return nil, err
				}
				mapOut[key] = v1
			default:
				return nil, fmt.Errorf(
					"YAML data contained a map key of type %t; only string keys are allowed",
					k)
			}
		}
		return mapOut, nil
	default:
		return data, nil
	}
}
