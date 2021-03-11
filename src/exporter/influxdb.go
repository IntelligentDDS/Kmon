package exporter

import (
	"time"

	client "github.com/influxdata/influxdb1-client/v2"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/config"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
	"go.uber.org/zap"
)

type influxdb struct {
	client client.Client
	logger *zap.Logger
}

func StartInfluxDB(conf config.ExpInfluxDB, logger *zap.Logger) (model.ExportFunc, error) {
	client, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     conf.Host,
		Username: conf.UserName,
		Password: conf.Password,
	})

	if err != nil {
		return nil, err
	}

	db := influxdb{
		client: client,
		logger: logger,
	}

	return db.Export, nil
}

func (db *influxdb) Export(database string, data []*model.ExportData) {
	logger := db.logger
	// TODO: 需要新版本influxdb
	// if hasInit, ok := exporter.Init[database]; !(ok && hasInit) {
	// 	createQuery := client.NewQuery(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", title), "", "")
	// 	if _, err := exporter.Client.Query(createQuery); err != nil {
	// 		logger.Info("Init database error:", zap.String("database", database), zap.Error(err))
	// 		return
	// 	}
	// 	exporter.Init[database] = true
	// }

	bp, err := client.NewBatchPoints(client.BatchPointsConfig{Database: database})

	if err != nil {
		logger.Error("", zap.Error(err))
		return
	}

	for _, point := range data {
		var pt *client.Point

		if tsi, ok := point.Fields["timestamp"]; ok {
			if ts, ok := tsi.(time.Time); ok {
				delete(point.Fields, "timestamp")
				pt, err = client.NewPoint(point.Name, point.Tags, point.Fields, ts)
			} else {
				pt, err = client.NewPoint(point.Name, point.Tags, point.Fields)
			}
		} else {
			pt, err = client.NewPoint(point.Name, point.Tags, point.Fields)
		}

		if err != nil {
			logger.Error("influxdb", zap.Error(err))
		}
		bp.AddPoint(pt)
	}

	err = db.client.Write(bp)
	if err != nil {
		logger.Error("influxdb", zap.Error(err))
	}
}
