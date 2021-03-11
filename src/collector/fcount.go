package collector

import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/iovisor/gobpf/bcc"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/collector/utils"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/config"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/info"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
	"go.uber.org/zap"
)

type fcount struct {
	mutex    sync.Mutex
	conf     config.ColFcount
	module   *bcc.Module
	countMap *bcc.Table
	getter   *info.Getter
	pidLis   *utils.PIDLis
}

// StartFcount ...
func StartFcount(confCh <-chan config.ColFcount, getter *info.Getter) error {
	logger := getter.Logger()

	// 初始化
	conf := <-confCh
	collector, err := setupFcountBCC(conf, getter)
	if err != nil {
		return err
	}
	collector.onConfigChange(conf)

	// 周期处理
	go func() {
		done := make(chan bool)
		getter.GetEvents().RegisterProgramEnd(func() { done <- true })

		for {
			collector.exportData(getter.GetDataCh())

			collector.mutex.Lock()
			period := collector.conf.ExportConfig.Period
			collector.mutex.Unlock()
			if waitPeriod(period, done) {
				return
			}
		}
	}()

	// 信号处理
	go func() {
		done := make(chan bool)
		getter.GetEvents().RegisterProgramEnd(func() { done <- true })
		for {
			select {
			case conf := <-confCh:
				collector.onConfigChange(conf)
			case addPID := <-collector.pidLis.AddCh:
				var val struct {
					Count    uint64
					Retcount uint64
				}
				pid := uint32(addPID)
				{
					collector.mutex.Lock()
					collector.countMap.SetP(unsafe.Pointer(&pid), unsafe.Pointer(&val))
					logger.Info("create", zap.Uint32("pid", pid))
					collector.mutex.Unlock()
				}
			case delPID := <-collector.pidLis.DelCh:
				pid := uint32(delPID)
				{
					collector.mutex.Lock()
					collector.countMap.DeleteP(unsafe.Pointer(&pid))
					logger.Info("delete", zap.Uint32("pid", pid))
					collector.mutex.Unlock()
				}

			case <-done:
				break
			}
		}
	}()

	return nil
}

func setupFcountBCC(conf config.ColFcount, getter *info.Getter) (*fcount, error) {
	logger := getter.Logger()
	module, err := loadModule("./bcc/fcount.c", logger)

	if err != nil {
		return nil, err
	}

	err = attachKprobe(module, "fcount_begin", conf.Function, false)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}

	if conf.CountReturn {
		err = attachKprobe(module, "fcount_return", conf.Function, true)
		if err != nil {
			logger.Error("", zap.Error(err))
			return nil, err
		}
	}

	// 不要在这里赋值conf，初始化时需要执行onConfigChange来进行PID的设置
	return &fcount{
		mutex:    sync.Mutex{},
		module:   module,
		conf:     config.ColFcount{},
		pidLis:   utils.NewPIDLis(getter, config.ListenConfig{}, 1024, logger),
		countMap: bcc.NewTable(module.TableId("data"), module),
		getter:   getter,
	}, nil
}

func (collector *fcount) exportData(dataCh chan<- model.ExportDataWarpper) {
	// logger := collector.getter.Logger()

	// 每个循环重新读取配置检查是否更新
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	conf := collector.conf

	// 遍历并统计函数计数
	data := []*model.ExportData{}
	iter := collector.countMap.Iter()
	for iter.Next() {
		key := iter.Key()
		val := iter.Leaf()

		// 通过指针直接操作，因为[]byte并不被看作指针，所以需要二阶解引用
		keyStruct := **(**uint32)(unsafe.Pointer(&key))
		valStruct := **(**struct {
			Count    uint64
			Retcount uint64
		})(unsafe.Pointer(&val))
		// logger.Info("", zap.Any("key", keyStruct), zap.Any("val", keyStruct))

		// 统一数据格式
		// logger.Info("fcount", zap.Any("config", conf.ExportConfig))
		exporterData := &model.ExportData{
			Name: conf.ExportConfig.DataName,
			Tags: map[string]string{
				"pid":       fmt.Sprint(keyStruct),
				"func_name": string(conf.Function),
			},
			Fields: map[string]interface{}{
				"timestamp": time.Now(),
				"count":     int(valStruct.Count), // 实验室部署的influxdb版本不支持uint类型
			},
		}
		// 补充额外数据
		containerName := collector.getter.GetContainerFromPID(int(keyStruct))
		if containerName != "" {
			exporterData.Tags["container"] = containerName
		}
		if conf.CountReturn {
			exporterData.Fields["ret_count"] = int(valStruct.Retcount) // 实验室部署的influxdb版本不支持uint类型
		}
		data = append(data, exporterData)

		// 重置计数
		valStruct.Count = 0
		valStruct.Retcount = 0
		collector.countMap.SetP(unsafe.Pointer(&keyStruct), unsafe.Pointer(&valStruct))
	}

	// 导出数据
	if len(data) > 0 {
		dataCh <- model.ExportDataWarpper{
			Data:     data,
			Exporter: conf.ExportConfig.ExporterConfig,
		}
	}
}

func (collector *fcount) onConfigChange(conf config.ColFcount) {
	// logger := collector.getter.Logger()
	// logger.Info("fcount", zap.String("change", "before"))
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	collector.pidLis.OnConfigChange(conf.ListenConfig)
	collector.conf = conf
	// logger.Info("fcount", zap.String("change", "after"))
}
