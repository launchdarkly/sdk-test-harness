package harness

import (
	"bytes"
	"encoding/json"
	"regexp"
)

// A \u escape for a UTF-16 surrogate code point (U+D800 to U+DFFF). JSON writers that escape non-ASCII
// characters write a character outside the Basic Multilingual Plane as a pair of these.
var surrogateEscapeRegex = regexp.MustCompile(`\\[uU][dD][89a-fA-F]`)

// normalizeSurrogateEscapes rewrites a JSON document that contains surrogate pair escapes (such as
// "\ud83d\ude00") so that those characters appear as plain UTF-8 instead. Some of the JSON decoders
// that the harness uses for test service responses (the ldvalue package, through go-jsonstream) decode
// each half of a surrogate pair separately as U+FFFD, so without this a test service whose JSON encoder
// escapes non-ASCII characters (as Python's json.dumps does by default) would appear to return corrupted
// strings. The input is returned unchanged if it has no surrogate escapes or is not valid JSON.
func normalizeSurrogateEscapes(data []byte) []byte {
	if !surrogateEscapeRegex.Match(data) {
		return data
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber() // keep numbers exactly as written
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return data
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return data
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}
