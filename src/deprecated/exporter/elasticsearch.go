package exporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	elasticsearch "github.com/elastic/go-elasticsearch/v7"
	"go.uber.org/zap"

	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
)

func init() {
	Factors["elasticsearch"] = NewElasticsearch
}

type Elasticsearch struct {
	Client *elasticsearch.Client
	Config *ElasticsearchConfig
	Lock   *sync.Mutex
}

type ElasticsearchConfig struct {
	Addr     string
	Username string
	Password string
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

func ParseConfig(config *model.ExporterConfig) (*ElasticsearchConfig, error) {
	cfg := ElasticsearchConfig{
		Addr:     "http://localhost:9200",
		Username: "",
		Password: "",
	}

	if data, ok := config.Config["addr"]; ok {
		if cfg.Addr, ok = data.(string); !ok {
			return nil, fmt.Errorf("%s is not string", "addr")
		}
	}

	if data, ok := config.Config["username"]; ok {
		if cfg.Username, ok = data.(string); !ok {
			return nil, fmt.Errorf("%s is not string", "username")
		}
	}

	if data, ok := config.Config["password"]; ok {
		if cfg.Password, ok = data.(string); !ok {
			return nil, fmt.Errorf("%s is not string", "password")
		}
	}

	return &cfg, nil
}

func NewElasticsearch(config *model.ExporterConfig) (model.Exporter, error) {
	esCfg, err := ParseConfig(config)
	if err != nil {
		return nil, fmt.Errorf("Parse config error: %w", err)
	}

	exporter := &Elasticsearch{
		Lock: &sync.Mutex{},
	}
	// TODO: 断线重连
	exporter.Client, err = elasticsearch.NewClient(elasticsearch.Config{
		Addresses: []string{esCfg.Addr},
		Username:  esCfg.Username,
		Password:  esCfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("create client error: %w", err)
	}

	return exporter, nil
}

func (exporter *Elasticsearch) Export(title string, data []*model.ExportData) error {
	// TODO: 是否需要考虑data中指针数据在上传过程中被修改的可能性？
	if len(data) <= 0 {
		return nil
	}

	go func() {
		exporter.Lock.Lock()
		defer exporter.Lock.Unlock()

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
				logger.Info("Error:", zap.Error(err))
				return
			}

		}
		res, err := exporter.Client.Bulk(bytes.NewReader(buffer.Bytes()), exporter.Client.Bulk.WithIndex(title))
		if err != nil {
			logger.Info("Error:", zap.Error(err))
			return
		}

		var raw map[string]interface{}
		var blk *bulkResponse
		if res.IsError() {
			if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
				logger.Info("Error:", zap.Error(err))
				return
			}

			logger.Info("Error:",
				zap.String("error", fmt.Sprintf("[%d] %s: %s",
					res.StatusCode,
					raw["error"].(map[string]interface{})["type"],
					raw["error"].(map[string]interface{})["reason"],
				)))
		} else {
			if err := json.NewDecoder(res.Body).Decode(&blk); err != nil {
				logger.Info("Error:", zap.Error(err))
				return
			}
			for _, d := range blk.Items {
				if d.Index.Status > 201 {
					logger.Error("Error",
						zap.String("err", fmt.Sprintf("[%d]: %s: %s: %s: %s", d.Index.Status,
							d.Index.Error.Type,
							d.Index.Error.Reason,
							d.Index.Error.Cause.Type,
							d.Index.Error.Cause.Reason,
						)),
					)
				}
			}
		}

		res.Body.Close()
		buffer.Reset()
	}()

	return nil
}
