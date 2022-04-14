package testdata

import (
	"embed"
	_ "embed"
	"fmt"
	"path/filepath"

	"github.com/launchdarkly/sdk-test-harness/framework/ldtest"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
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
// The path parameter is relative to testdata/data-files.
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
// The path parameter is relative to testdata/data-files.
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

// LoadAndParseAllTestSuites calls LoadAllDataFiles and then parses each of the resulting SourceInfos
// as JSON or YAML into the specified type.
func LoadAndParseAllTestSuites[V any](t *ldtest.T, dirName string) []V {
	sources, err := LoadAllDataFiles(dirName)
	require.NoError(t, err)

	ret := make([]V, 0, len(sources))
	for _, source := range sources {
		var suite V
		if err := ParseJSONOrYAML(source.Data, &suite); err != nil {
			require.NoError(t, fmt.Errorf("error parsing %q %s: %w", source.BaseName, source.ParamsString(), err))
		}
		ret = append(ret, suite)
	}
	return ret
}
