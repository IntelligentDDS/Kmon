package config

import (
	"os"

	"gopkg.in/yaml.v2"
)

var configure Config

type Config struct {
	Exporters Exporter  `yaml:"exporter,omitempty"`
	Collector Collector `yaml:"collector,omitempty"`
	Provide   Provider  `yaml:"provider,omitempty"`
}

type Exporter struct {
	InfluxDB      []ExpInfluxDB      `yaml:"influxdb"`
	Elasticsearch []ExpElasticsearch `yaml:"elasticsearch"`
}

type Collector struct {
	Fcount  []ColFcount  `yaml:"fcount"`
	Tcpreq  []ColTcpreq  `yaml:"tcpreq"`
	Biostat []ColBiostat `yaml:"biostat"`
	Tcpdrop []ColTcpdrop `yaml:"tcpdrop"`
	Offcpu  []ColOffcpu  `yaml:"offcpu"`
}

type Provider struct {
}

type ExportConfig struct {
	Period         int              `yaml:"period"`
	DataName       string           `yaml:"data_name"`
	ExporterConfig []ExporterConfig `yaml:"exporter"`
}

type ExporterConfig struct {
	Name     string `yaml:"name"`
	Database string `yaml:"database"`
}

type ListenConfig struct {
	PIDGroup []int    `yaml:"pid"`
	K8sGroup []string `yaml:"k8s"`
}

func FindConfig(path string) (conf Config, err error) {
	configFile, err := os.Open(path)
	if err != nil {
		return
	}

	conf = Config{}
	err = yaml.NewDecoder(configFile).Decode(&conf)

	return
}
