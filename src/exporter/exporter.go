package exporter

import (
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/info"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
	"go.uber.org/zap"
)

func Setup(getter *info.Getter, logger *zap.Logger) {
	conf := getter.GetConfiguration().Exporters

	exporterMap := make(map[string]model.ExportFunc)

	for _, influx := range conf.InfluxDB {
		function, err := StartInfluxDB(influx, logger)
		if err != nil {
			logger.Error("", zap.Error(err))
			continue
		}
		exporterMap[influx.Name] = function
	}

	// TODO: support config change
	go func() {
		dataCh := getter.GetDataCh()
		for {
			data := <-dataCh
			for _, exporter := range data.Exporter {
				database := exporter.Database
				if database == "" {
					database = "Unknown"
				}

				if f, ok := exporterMap[exporter.Name]; ok {
					go f(database, data.Data)
				}
			}
		}
	}()
}
