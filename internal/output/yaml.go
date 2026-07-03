package output

import "gopkg.in/yaml.v3"

// YAMLFormatter renders data as YAML with 2-space indentation.
// No colorization is applied.
type YAMLFormatter struct{}

func (f *YAMLFormatter) Name() string { return "yaml" }

func (f *YAMLFormatter) Format(data any, cfg FormatConfig) ([]byte, error) {
	out, err := yaml.Marshal(data)
	if err != nil {
		return nil, err
	}
	return out, nil
}