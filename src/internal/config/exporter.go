package config

type ExpElasticsearch struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	UserName string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

type ExpInfluxDB struct {
	Name     string `yaml:"name"`
	Host     string `yaml:"host"`
	UserName string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}
