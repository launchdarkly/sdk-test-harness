package testmodel

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	yaml "gopkg.in/yaml.v3"
)

type SubstitutionSet map[string]ldvalue.Value

type SourceInfo struct {
	FilePath string
	BaseName string
	Params   SubstitutionSet
	Data     []byte
}

func (s SourceInfo) ParseInto(target interface{}) error {
	if err := ParseJSONOrYAML(s.Data, target); err != nil {
		return fmt.Errorf("error parsing %q %s: %w", s.BaseName, s.ParamsString(), err)
	}
	return nil
}

func (s SourceInfo) ParamsString() string {
	if len(s.Params) == 0 {
		return ""
	}
	ps := ""
	for k, v := range s.Params {
		if ps != "" {
			ps += ","
		}
		ps += k + "=" + v.String()
	}
	return "(" + ps + ")"
}

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

func ExpandSubstitutions(originalData []byte) ([]SourceInfo, error) {
	var substs struct {
		Constants  SubstitutionSet   `json:"constants"`
		Parameters []json.RawMessage `json:"parameters"`
	}
	if err := ParseJSONOrYAML(originalData, &substs); err != nil {
		return nil, err
	}
	if len(substs.Constants) == 0 && len(substs.Parameters) == 0 {
		return []SourceInfo{
			{Data: originalData},
		}, nil
	}
	parameterSets, err := makeParameterPermutations(substs.Parameters)
	if err != nil {
		return nil, err
	}
	if len(parameterSets) == 0 {
		return []SourceInfo{
			{Data: replaceVariables(originalData, substs.Constants)},
		}, nil
	}
	ret := make([]SourceInfo, 0, len(parameterSets))
	for _, paramsSet := range parameterSets {
		transformed := replaceVariables(originalData, substs.Constants)
		transformed = replaceVariables(transformed, paramsSet)
		transformed = replaceVariables(transformed, substs.Constants)
		ret = append(ret, SourceInfo{Data: transformed, Params: paramsSet})
	}
	return ret, nil
}

func makeParameterPermutations(paramsData []json.RawMessage) ([]SubstitutionSet, error) {
	if len(paramsData) == 0 {
		return nil, nil
	}
	allData, _ := json.Marshal(paramsData)
	if ldvalue.Parse(paramsData[0]).Type() == ldvalue.ObjectType {
		var list []SubstitutionSet
		if err := json.Unmarshal(allData, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	if ldvalue.Parse(paramsData[0]).Type() != ldvalue.ArrayType {
		return nil, errors.New("unable to parse parameters - must be an array of objects or an array of arrays")
	}
	var lists [][]SubstitutionSet
	if err := json.Unmarshal(allData, &lists); err != nil {
		return nil, err
	}
	indices := make([]int, len(lists))
	var result []SubstitutionSet
	for {
		mergedSet := make(SubstitutionSet)
		for i := 0; i < len(lists); i++ {
			thisSet := lists[i][indices[i]]
			for k, v := range thisSet {
				mergedSet[k] = v
			}
		}
		result = append(result, mergedSet)
		incrementPos := 0
		for incrementPos < len(lists) {
			indices[incrementPos]++
			if indices[incrementPos] < len(lists[incrementPos]) {
				break
			}
			indices[incrementPos] = 0
			incrementPos++
		}
		if incrementPos == len(lists) {
			return result, nil
		}
	}
}

func replaceVariables(originalData []byte, substs SubstitutionSet) []byte {
	str := string(originalData)
	str = strings.ReplaceAll(str, `\u003c`, "<")
	str = strings.ReplaceAll(str, `\u003e`, ">")
	for name, value := range substs {
		typedValueStr := value.JSONString()
		str = strings.ReplaceAll(str, `"<`+name+`>"`, typedValueStr)
		interpolatedValueStr := typedValueStr
		if value.IsString() {
			interpolatedValueStr = value.StringValue()
		}
		str = strings.ReplaceAll(str, "<"+name+">", interpolatedValueStr)
	}
	return []byte(str)
}

func ReadFile(path string) ([]SourceInfo, error) {
	ret := make([]SourceInfo, 0, 10)
	data, err := os.ReadFile(path) //nolint:gosec // yes, we know the file path is a variable
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %w", path, err)
	}
	baseName := filepath.Base(path)
	sources, err := ExpandSubstitutions(data)
	if err != nil {
		return nil, fmt.Errorf("error reading %q: %s", path, err)
	}
	for _, source := range sources {
		source.FilePath = path
		source.BaseName = baseName
		ret = append(ret, source)
	}
	return ret, nil
}

func ReadFileSingle(path string) (SourceInfo, error) {
	sources, err := ReadFile(path)
	if err != nil {
		return SourceInfo{}, err
	}
	if len(sources) != 1 {
		return SourceInfo{}, fmt.Errorf("expected a single set of data in %q", path)
	}
	return sources[0], nil
}

func ReadAllFiles(path string) ([]SourceInfo, error) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var ret []SourceInfo
	for _, file := range files {
		filePath := filepath.Join(path, file.Name())
		sources, err := ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		ret = append(ret, sources...)
	}
	return ret, nil
}
