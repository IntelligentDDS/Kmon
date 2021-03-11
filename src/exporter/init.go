package exporter

// import (
// 	"fmt"

// 	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
// 	"go.uber.org/zap"
// )

// var logger, _ = zap.NewDevelopment()
// var exporters map[string]model.Exporter = make(map[string]model.Exporter)
// var Factors map[string]func(*model.ExporterConfig) (model.Exporter, error) = make(map[string]func(*model.ExporterConfig) (model.Exporter, error))

// func RegisterExporter(config *model.ExporterConfig) error {
// 	factor, ok := Factors[config.Type]
// 	if !ok {
// 		return fmt.Errorf("Cannot find exporter type: %s", config.Type)
// 	}

// 	exporter, err := factor(config)
// 	if err != nil {
// 		return err
// 	}
// 	exporters[config.Name] = exporter
// 	return nil
// }

// func GetExporter(name string) model.Exporter {
// 	exporter, ok := exporters[name]
// 	if !ok {
// 		return &ErrorExporter{Name: name}
// 	}

// 	return exporter
// }

// type ErrorExporter struct {
// 	Name string
// }

// func (exporter *ErrorExporter) Export(title string, data []*model.ExportData) error {
// 	return fmt.Errorf("Cannot find exporter: %s", exporter.Name)
// }
