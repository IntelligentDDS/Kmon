package model

import "gitlab.dds-sysu.tech/Wuny/kmon/src/internal/config"

type ExportFunc = func(database string, data []*ExportData)

type Exporter interface {
	Export(string, []*ExportData) error
}

type ExportData struct {
	Name       string
	Tags       map[string]string
	Fields     map[string]interface{}
	Additional map[string]interface{}
}

type ExportDataWarpper struct {
	Data     []*ExportData
	Exporter []config.ExporterConfig
}
