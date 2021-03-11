package exporter

import (
	"bytes"
	"encoding/json"
	"fmt"

	elasticsearch "github.com/elastic/go-elasticsearch/v7"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/config"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
	"go.uber.org/zap"
)

type elastic struct {
	client *elasticsearch.Client
	logger *zap.Logger
}

type bulkResponse struct {
	Errors bool `json:"errors"`
	Items  []struct {
		Index struct {
			ID     string `json:"_id"`
			Result string `json:"result"`
			Status int    `json:"status"`
			Error  struct {
				Type   string `json:"type"`
				Reason string `json:"reason"`
				Cause  struct {
					Type   string `json:"type"`
					Reason string `json:"reason"`
				} `json:"caused_by"`
			} `json:"error"`
		} `json:"index"`
	} `json:"items"`
}

func StartElasticsearch(conf config.ExpElasticsearch, logger *zap.Logger) (model.ExportFunc, error) {
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{conf.Host},
		Username:  conf.UserName,
		Password:  conf.Password,
	})

	if err != nil {
		return nil, err
	}

	db := elastic{
		client: client,
		logger: logger,
	}

	return db.Export, nil
}

func (db *elastic) Export(database string, data []*model.ExportData) {
	logger := db.logger

	var buffer bytes.Buffer
	for _, point := range data {
		meta := []byte(fmt.Sprintf(`{ "index" : { "_index" : "%s" } }%s`, point.Name, "\n"))

		totalData := make(map[string]interface{})
		for k, v := range point.Fields {
			totalData[k] = v
		}
		for k, v := range point.Tags {
			totalData[k] = v
		}

		if len(totalData) == 0 {
			continue
		}

		if data, err := json.Marshal(totalData); err == nil {
			data = append(data, "\n"...)
			buffer.Grow(len(meta) + len(data))
			buffer.Write(meta)
			buffer.Write(data)
		} else {
			logger.Error("elasticsearch", zap.Error(err))
			return
		}

	}
	res, err := db.client.Bulk(bytes.NewReader(buffer.Bytes()), db.client.Bulk.WithIndex(database))
	if err != nil {
		logger.Error("elasticsearch", zap.Error(err))
		return
	}
	defer res.Body.Close()

	var raw map[string]interface{}
	var blk *bulkResponse
	if res.IsError() {
		if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
			logger.Info("elasticsearch", zap.Error(err))
			return
		}

		logger.Error("elasticsearch",
			zap.String("error", fmt.Sprintf("[%d] %s: %s",
				res.StatusCode,
				raw["error"].(map[string]interface{})["type"],
				raw["error"].(map[string]interface{})["reason"],
			)))
	} else {
		if err := json.NewDecoder(res.Body).Decode(&blk); err != nil {
			logger.Error("Error:", zap.Error(err))
			return
		}
		for _, d := range blk.Items {
			if d.Index.Status > 201 {
				logger.Error("elasticsearch",
					zap.String("error", fmt.Sprintf("[%d]: %s: %s: %s: %s", d.Index.Status,
						d.Index.Error.Type,
						d.Index.Error.Reason,
						d.Index.Error.Cause.Type,
						d.Index.Error.Cause.Reason,
					)),
				)
			}
		}
	}
}
