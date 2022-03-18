package data

import (
	"embed"
	"fmt"
	"path/filepath"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

//go:embed data-files
var dataFilesRoot embed.FS

const dataBasePath = "data-files"

// SourceInfo represents JSON or YAML data that was read from a file, after post-processing to expand
// constants and parameters. For non-parameterized tests, you will get one SourceInfo per file. For
// parameterized tests, there can be many instances per file, each with its own version of Data.
// See ReadFile and docs/data_files.md.
type SourceInfo struct {
	FilePath string
	BaseName string
	Params   map[string]ldvalue.Value
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

// LoadDataFile reads a data file and performs any necessary constant/parameter substitutions. It can
// return more than one SourceInfo because any file can be a parameterized test. See docs/data_files.md.
//
// The path parameter is relative to data/data-files.
func LoadDataFile(path string) ([]SourceInfo, error) {
	ret := make([]SourceInfo, 0, 10) // preallocate a little because it's likely there will be multiple results
	data, err := dataFilesRoot.ReadFile(dataBasePath + "/" + path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %w", path, err)
	}
	baseName := filepath.Base(path)
	sources, err := expandSubstitutions(data)
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

// LoadAllDataFiles reads all data files in a directory and performs any necessary constant/parameter
// substitutions. It can return more than one SourceInfo per file, because any file can be a parameterized
// test. See docs/data_files.md.
//
// The path parameter is relative to data/data-files.
func LoadAllDataFiles(path string) ([]SourceInfo, error) {
	files, err := dataFilesRoot.ReadDir(dataBasePath + "/" + path)
	if err != nil {
		return nil, err
	}
	var ret []SourceInfo
	for _, file := range files {
		filePath := path + "/" + file.Name()
		sources, err := LoadDataFile(filePath)
		if err != nil {
			return nil, err
		}
		ret = append(ret, sources...)
	}
	return ret, nil
}
