package config

type ColFcount struct {
	Name        string `yaml:"name"`
	Function    string `yaml:"function"`
	CountReturn bool   `yaml:"count_return"`

	ListenConfig ListenConfig `yaml:"listen"`
	ExportConfig ExportConfig `yaml:"export"`
}
type ColTcpreq struct {
	Name    string `yaml:"name"`
	Verbose uint32 `yaml:"verbose"`

	ListenConfig ListenConfig `yaml:"listen"`
	ExportConfig ExportConfig `yaml:"export"`
}
type ColBiostat struct {
	Name     string `yaml:"name"`
	Verbose  uint32 `yaml:"verbose"`
	BinScale uint64 `yaml:"bin_scale"`

	ListenConfig ListenConfig `yaml:"listen"`
	ExportConfig ExportConfig `yaml:"export"`
}
type ColTcpdrop struct {
	Name    string `yaml:"name"`
	Verbose uint32 `yaml:"verbose"`

	ListenConfig ListenConfig `yaml:"listen"`
	ExportConfig ExportConfig `yaml:"export"`
}
type ColOffcpu struct {
	Name    string `yaml:"name"`
	Verbose uint32 `yaml:"verbose"`
	State   uint32 `yaml:"state"`

	TimeStart uint32 `yaml:"time_start"`
	TimeEnd   uint32 `yaml:"time_end"`

	ListenConfig ListenConfig `yaml:"listen"`
	ExportConfig ExportConfig `yaml:"export"`
}
