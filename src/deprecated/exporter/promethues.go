package exporter

import (
	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
)

// TODO: 完成Promethues的导出方式
func init() {
	Factors["prometheus"] = NewPromethues
}

type Prometheus struct{}

type PrometheusConfig struct{}

func NewPromethues(config *model.ExporterConfig) (model.Exporter, error) {
	return nil, nil
}

func (exporter *Prometheus) Export(title string, data []*model.ExportData) error {
	return nil
}
