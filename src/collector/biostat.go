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

type biostat struct {
	mutex      sync.Mutex
	conf       config.ColBiostat
	module     *bcc.Module
	controlMap *bcc.Table
	countMap   *bcc.Table
	distMap    *bcc.Table
	getter     *info.Getter
	pidLis     *utils.PIDLis
}

type bioKey struct {
	PID    uint32
	RWFlag uint64
	Name   [32]byte
}

type bioVal struct {
	Bytes uint64
	Ns    uint64
}

type bioHistKey struct {
	Slot   uint32
	PID    uint32
	RWFlag uint64
	Name   [32]byte // TODO: change into major and mior of device
}

type bioHistVal struct {
	Slots map[uint32]uint64
	Scale uint64
}

type bioControlMap struct {
	Verbose  uint32
	BinScale uint64
}

// StartBiostat ...
func StartBiostat(confCh <-chan config.ColBiostat, getter *info.Getter) error {
	logger := getter.Logger()

	// 初始化
	conf := <-confCh
	collector, err := setupBioBCC(conf, getter)
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
			collector.exportHistData(getter.GetDataCh())

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
				collector.mutex.Lock()
				con := bioControlMap{
					Verbose:  collector.conf.Verbose,
					BinScale: collector.conf.BinScale,
				}
				collector.mutex.Unlock()

				pid := uint32(addPID)
				{
					collector.mutex.Lock()
					collector.controlMap.SetP(unsafe.Pointer(&pid), unsafe.Pointer(&con))
					logger.Info("create", zap.Uint32("pid", pid))
					collector.mutex.Unlock()
				}
			case delPID := <-collector.pidLis.DelCh:
				pid := uint32(delPID)
				{
					collector.mutex.Lock()
					collector.controlMap.DeleteP(unsafe.Pointer(&pid))
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

func setupBioBCC(conf config.ColBiostat, getter *info.Getter) (*biostat, error) {
	logger := getter.Logger()
	module, err := loadModule("./bcc/biostat.c", logger)
	if err != nil {
		return nil, err
	}

	err = attachKprobe(module, "trace_pid_start", "blk_account_io_start", false)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}
	err = attachKprobe(module, "trace_req_start", "blk_mq_start_request", false)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}
	err = attachKprobe(module, "trace_req_completion", "blk_account_io_done", false)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}

	// 不要在这里赋值conf，初始化时需要执行onConfigChange来进行PID的设置
	return &biostat{
		mutex:      sync.Mutex{},
		module:     module,
		conf:       config.ColBiostat{},
		pidLis:     utils.NewPIDLis(getter, config.ListenConfig{}, 1024, logger),
		controlMap: bcc.NewTable(module.TableId("control_map"), module),
		countMap:   bcc.NewTable(module.TableId("counts"), module),
		distMap:    bcc.NewTable(module.TableId("dist"), module),
		getter:     getter,
	}, nil
}

func (collector *biostat) exportData(dataCh chan<- model.ExportDataWarpper) {
	logger := collector.getter.Logger()

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
		keyStruct := **(**bioKey)(unsafe.Pointer(&key))
		valStruct := **(**bioVal)(unsafe.Pointer(&val))

		// 统一数据格式
		// logger.Info("fcount", zap.Any("config", conf.ExportConfig))
		exporterData := &model.ExportData{
			Name: conf.ExportConfig.DataName,
			Tags: map[string]string{
				"pid":      fmt.Sprint(keyStruct.PID),
				"flag":     parseRWFlag(keyStruct.RWFlag),
				"diskName": ctypeByteToString(keyStruct.Name[:]),
			},
			Fields: map[string]interface{}{
				"period":            conf.ExportConfig.Period,
				"timestamp":         time.Now(),
				"I/O(total,Bytes)":  int(valStruct.Bytes),
				"Latency(total,ns)": int(valStruct.Ns),
			},
		}
		// 补充额外数据
		containerName := collector.getter.GetContainerFromPID(int(keyStruct.PID))
		if containerName != "" {
			exporterData.Tags["container"] = containerName
		}
		data = append(data, exporterData)

		if valStruct.Bytes > 0 {
			logger.Info("biostat", zap.Any("data", exporterData))
		}

		collector.countMap.DeleteP(unsafe.Pointer(&keyStruct))
	}

	// 导出数据
	if len(data) > 0 {
		dataCh <- model.ExportDataWarpper{
			Data:     data,
			Exporter: conf.ExportConfig.ExporterConfig,
		}
	}
}

func (collector *biostat) exportHistData(dataCh chan<- model.ExportDataWarpper) {
	// logger := collector.getter.Logger()

	// 每个循环重新读取配置检查是否更新
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	conf := collector.conf

	// 遍历并统计函数计数
	data := []*model.ExportData{}
	iter := collector.distMap.Iter()

	heatmapData := map[bioKey]bioHistVal{}
	for iter.Next() {
		key := iter.Key()
		val := iter.Leaf()

		// 通过指针直接操作，因为[]byte并不被看作指针，所以需要二阶解引用
		keyStruct := **(**bioHistKey)(unsafe.Pointer(&key))
		valStruct := **(**uint64)(unsafe.Pointer(&val))

		bioKeyStruct := bioKey{
			PID:    keyStruct.PID,
			RWFlag: keyStruct.RWFlag,
			Name:   keyStruct.Name,
		}

		bioKeyVal, ok := heatmapData[bioKeyStruct]
		if !ok {
			bioKeyVal = bioHistVal{
				Slots: make(map[uint32]uint64),
				Scale: conf.BinScale,
			}
			heatmapData[bioKeyStruct] = bioKeyVal
		}
		bioKeyVal.Slots[keyStruct.Slot] = valStruct

		// 重置计数
		collector.distMap.DeleteP(unsafe.Pointer(&keyStruct))
	}

	for key, val := range heatmapData {
		// 统一数据格式
		exporterData := &model.ExportData{
			Name: conf.ExportConfig.DataName,
			Tags: map[string]string{
				"pid":      fmt.Sprint(key.PID),
				"flag":     parseRWFlag(key.RWFlag),
				"diskName": ctypeByteToString(key.Name[:]),
			},
			Fields: map[string]interface{}{},
		}
		exporterData.Fields["binScale"] = int(val.Scale) // 实验室部署的influxdb版本不支持uint类型
		for slotIdx, count := range val.Slots {
			exporterData.Fields[fmt.Sprintf("latencyBin(%d)", slotIdx)] = int(count) // 实验室部署的influxdb版本不支持uint类型
		}

		// 补充额外数据
		containerName := collector.getter.GetContainerFromPID(int(key.PID))
		if containerName != "" {
			exporterData.Tags["container"] = containerName
		}

		data = append(data, exporterData)
	}

	// 导出数据
	if len(data) > 0 {
		dataCh <- model.ExportDataWarpper{
			Data:     data,
			Exporter: conf.ExportConfig.ExporterConfig,
		}
	}
}

func (collector *biostat) onConfigChange(conf config.ColBiostat) {
	logger := collector.getter.Logger()
	// logger.Info("fcount", zap.String("change", "before"))
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	collector.pidLis.OnConfigChange(conf.ListenConfig)
	collector.conf = conf

	iter := collector.controlMap.Iter()
	for iter.Next() {
		key := iter.Key()
		val := iter.Leaf()

		// 通过指针直接操作，因为[]byte并不被看作指针，所以需要二阶解引用
		keyStruct := **(**uint32)(unsafe.Pointer(&key))
		valStruct := **(**bioControlMap)(unsafe.Pointer(&val))
		logger.Info("biostat", zap.Any("val", valStruct))

		// 修改详细等级
		valStruct.Verbose = uint32(conf.Verbose)
		valStruct.BinScale = conf.BinScale
		collector.controlMap.SetP(unsafe.Pointer(&keyStruct), unsafe.Pointer(&valStruct))
	}
	// logger.Info("fcount", zap.String("change", "after"))
}

func parseRWFlag(flag uint64) string {
	switch flag {
	case 0:
		return "Read"
	case 1:
		return "Write"
	}

	return "Unknown"
}

func ctypeByteToString(s []byte) string {
	for idx, b := range s {
		if b == '\u0000' {
			return string(s[:idx])
		}
	}
	return string(s)
}
