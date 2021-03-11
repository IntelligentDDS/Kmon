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

type offcpu struct {
	mutex          sync.Mutex
	conf           config.ColOffcpu
	module         *bcc.Module
	controlMap     *bcc.Table
	countMap       *bcc.Table
	stackMap       *bcc.Table
	cpuEventCh     chan []byte
	cpuEventLostCh chan uint64
	cpuEventBuffer []*model.ExportData
	getter         *info.Getter
	pidLis         *utils.PIDLis
}

type offcpuControlMap struct {
	Verbose   uint32
	State     uint32
	TimeStart uint32
	TimeEnd   uint32
}

type tcpWarnEvent struct {
	PID       uint32
	TGID      uint32
	StartTime uint32
	EndTime   uint32
}

type offcpuKey struct {
	PID         uint32
	TGID        uint32
	UserStackID uint32
	StackID     uint32
	Name        [32]byte //TASK_COMM_LEN
}

// StartOffcpu ...
func StartOffcpu(confCh <-chan config.ColOffcpu, getter *info.Getter) error {
	logger := getter.Logger()

	// 初始化
	conf := <-confCh
	collector, err := setupOffcpuBCC(conf, getter)
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
			case l := <-collector.cpuEventLostCh:
				logger.Warn("offcpu", zap.Uint64("lost_event", l))
			case e := <-collector.cpuEventCh:
				val := **(**tcpWarnEvent)(unsafe.Pointer(&e))
				collector.bufferEvent(val)
			case addPID := <-collector.pidLis.AddCh:
				collector.mutex.Lock()
				con := offcpuControlMap{}
				con.Verbose = conf.Verbose
				con.State = conf.State
				con.TimeStart = conf.TimeStart
				con.TimeEnd = conf.TimeEnd
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

func setupOffcpuBCC(conf config.ColOffcpu, getter *info.Getter) (*offcpu, error) {
	logger := getter.Logger()
	module, err := loadModule("./bcc/offcpu.c", logger)
	if err != nil {
		return nil, err
	}

	err = attachKprobe(module, "oncpu", "finish_task_switch", false)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}

	cpuEventLostCh := make(chan uint64)
	cpuEventCh := make(chan []byte)
	cpuEventTable := bcc.NewTable(module.TableId("warn_events"), module)
	cpuEventMap, err := bcc.InitPerfMap(cpuEventTable, cpuEventCh, cpuEventLostCh)
	if err != nil {
		return nil, err
	}
	cpuEventMap.Start()

	// 不要在这里赋值conf，初始化时需要执行onConfigChange来进行PID的设置
	return &offcpu{
		mutex:          sync.Mutex{},
		module:         module,
		conf:           config.ColOffcpu{},
		pidLis:         utils.NewPIDLis(getter, config.ListenConfig{}, 1024, logger),
		controlMap:     bcc.NewTable(module.TableId("control_map"), module),
		countMap:       bcc.NewTable(module.TableId("counts"), module),
		stackMap:       bcc.NewTable(module.TableId("stack_traces"), module),
		cpuEventCh:     cpuEventCh,
		cpuEventLostCh: cpuEventLostCh,
		getter:         getter,
	}, nil
}

func (collector *offcpu) bufferEvent(e tcpWarnEvent) {
	// logger := collector.getter.Logger()
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	// 记录事件
	conf := collector.conf
	data := &model.ExportData{
		Name: conf.ExportConfig.DataName,
		Tags: map[string]string{
			"pid":  fmt.Sprint(e.PID),
			"tgid": fmt.Sprint(e.TGID),
			"type": "warn",
		},
		Fields: map[string]interface{}{
			"startTime": e.StartTime,
			"endTime":   e.EndTime,
		},
	}

	containerName := collector.getter.GetContainerFromPID(int(e.PID))
	if containerName != "" {
		data.Tags["container"] = containerName
	}

	collector.cpuEventBuffer = append(collector.cpuEventBuffer, data)
}

func (collector *offcpu) exportData(dataCh chan<- model.ExportDataWarpper) {
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
		keyStruct := **(**offcpuKey)(unsafe.Pointer(&key))
		valStruct := **(**uint64)(unsafe.Pointer(&val))

		// 统一数据格式
		// logger.Info("fcount", zap.Any("config", conf.ExportConfig))
		exporterData := &model.ExportData{
			Name: conf.ExportConfig.DataName,
			Tags: map[string]string{
				"pid":  fmt.Sprint(keyStruct.PID),
				"tgid": fmt.Sprint(keyStruct.TGID),
				"name": ctypeByteToString(keyStruct.Name[:]),
			},
			Fields: map[string]interface{}{
				"timestamp":     time.Now(),
				"dutration(us)": int64(valStruct), // 实验室部署的influxdb版本不支持uint类型
			},
		}
		// 补充额外数据
		containerName := collector.getter.GetContainerFromPID(int(keyStruct.PID))
		if containerName != "" {
			exporterData.Tags["container"] = containerName
		}
		if keyStruct.StackID != 0 {
			if stackStr, err := exportStack(collector.stackMap, keyStruct.StackID, true); err == nil {
				exporterData.Fields["kernelStack"] = stackStr
			}
		}
		if keyStruct.UserStackID != 0 {
			if stackStr, err := exportStack(collector.stackMap, keyStruct.StackID, false); err == nil {
				exporterData.Fields["userStack"] = stackStr
			}
		}
		data = append(data, exporterData)

		// 重置计数
		collector.countMap.DeleteP(unsafe.Pointer(&keyStruct))
	}

	// 导出数据
	if len(data) > 0 {
		// logger.Info("Offcpu", zap.Any("data", data))
		dataCh <- model.ExportDataWarpper{
			Data:     data,
			Exporter: conf.ExportConfig.ExporterConfig,
		}
	}
}

func (collector *offcpu) onConfigChange(conf config.ColOffcpu) {
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
		valStruct := **(**offcpuControlMap)(unsafe.Pointer(&val))
		logger.Info("Offcpu", zap.Any("val", valStruct))

		// 修改详细等级
		valStruct.Verbose = conf.Verbose
		valStruct.State = conf.State
		valStruct.TimeStart = conf.TimeStart
		valStruct.TimeEnd = conf.TimeEnd

		collector.controlMap.SetP(unsafe.Pointer(&keyStruct), unsafe.Pointer(&valStruct))
	}
	// logger.Info("fcount", zap.String("change", "after"))
}
