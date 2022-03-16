package data

import (
	"encoding/json"
	"errors"
	"strings"

	"gopkg.in/launchdarkly/go-sdk-common.v3/ldvalue"
)

type substitutionSet map[string]ldvalue.Value

func expandSubstitutions(originalData []byte) ([]SourceInfo, error) {
	var substs struct {
		Constants  substitutionSet   `json:"constants"`
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

func makeParameterPermutations(paramsData []json.RawMessage) ([]substitutionSet, error) {
	if len(paramsData) == 0 {
		return nil, nil
	}
	allData, _ := json.Marshal(paramsData)
	if ldvalue.Parse(paramsData[0]).Type() == ldvalue.ObjectType {
		var list []substitutionSet
		if err := json.Unmarshal(allData, &list); err != nil {
			return nil, err
		}
		return list, nil
	}
	if ldvalue.Parse(paramsData[0]).Type() != ldvalue.ArrayType {
		return nil, errors.New("unable to parse parameters - must be an array of objects or an array of arrays")
	}
	var lists [][]substitutionSet
	if err := json.Unmarshal(allData, &lists); err != nil {
		return nil, err
	}
	indices := make([]int, len(lists))
	var result []substitutionSet
	for {
		mergedSet := make(substitutionSet)
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

func replaceVariables(originalData []byte, substs substitutionSet) []byte {
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
