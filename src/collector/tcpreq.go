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

type tcpreq struct {
	mutex           sync.Mutex
	conf            config.ColTcpreq
	module          *bcc.Module
	controlMap      *bcc.Table
	tcpEventRecvCh  chan []byte
	tcpEventLostCh  chan uint64
	tcpEventMachine map[tcpConn]tcpEvent
	tcpEventBuffer  []*model.ExportData
	getter          *info.Getter
	pidLis          *utils.PIDLis
}

type tcpConn struct {
	peerAddr uint32
	myAddr   uint32
	peerPort uint16
	myPort   uint16
}

type tcpConnState struct {
	beginTs          time.Time
	beginMachineTsNs uint64
	RecvMachineTsNs  uint64
}

type tcpEvent struct {
	CurTs uint64

	Type uint32
	PID  uint32

	PeerAddr uint32
	MyAddr   uint32
	PeerPort uint16
	MyPort   uint16
}

type tcpControlMap struct {
	Verbose uint32
}

// StartTcpreq ...
func StartTcpreq(confCh <-chan config.ColTcpreq, getter *info.Getter) error {
	logger := getter.Logger()

	// 初始化
	conf := <-confCh
	collector, err := setupTcpreqBCC(conf, getter)
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
			case e := <-collector.tcpEventRecvCh:
				val := **(**tcpEvent)(unsafe.Pointer(&e))
				collector.bufferEvent(val)
				// logger.Info("tcpreq", zap.Any("event", val))
			case l := <-collector.tcpEventLostCh:
				logger.Warn("tcpreq", zap.Uint64("lost_event", l))
			case addPID := <-collector.pidLis.AddCh:

				collector.mutex.Lock()
				val := tcpControlMap{
					Verbose: collector.conf.Verbose,
				}
				collector.mutex.Unlock()

				pid := uint32(addPID)
				{
					collector.mutex.Lock()
					collector.controlMap.SetP(unsafe.Pointer(&pid), unsafe.Pointer(&val))
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

func setupTcpreqBCC(conf config.ColTcpreq, getter *info.Getter) (*tcpreq, error) {
	logger := getter.Logger()
	module, err := loadModule("./bcc/tcpreq.c", logger)

	if err != nil {
		return nil, err
	}

	err = attachKprobe(module, "kprobe_tcp_sendmsg_entry", "tcp_sendmsg", false)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}
	err = attachKprobe(module, "kprobe_tcp_cleanup_rbuf_entry", "tcp_cleanup_rbuf", false)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}
	err = attachKprobe(module, "kprobe_tcp_close", "tcp_close", false)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}
	err = attachKprobe(module, "kprobe_security_socket_accept", "security_socket_accept", false)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}
	err = attachKprobe(module, "kretprobe_move_addr_to_user", "move_addr_to_user", true)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}

	tcpEventRecvCh := make(chan []byte)
	tcpEventLostCh := make(chan uint64)
	tcpEventTable := bcc.NewTable(module.TableId("tcp_event"), module)
	perfmap, err := bcc.InitPerfMap(tcpEventTable, tcpEventRecvCh, tcpEventLostCh)
	if err != nil {
		return nil, err
	}
	perfmap.Start()
	// 不要在这里赋值conf，初始化时需要执行onConfigChange来进行PID的设置
	return &tcpreq{
		mutex:           sync.Mutex{},
		module:          module,
		tcpEventRecvCh:  tcpEventRecvCh,
		tcpEventLostCh:  tcpEventLostCh,
		tcpEventMachine: make(map[tcpConn]tcpEvent),
		conf:            config.ColTcpreq{},
		pidLis:          utils.NewPIDLis(getter, config.ListenConfig{}, 1024, logger),
		controlMap:      bcc.NewTable(module.TableId("control_map"), module),
		getter:          getter,
	}, nil
}

func (collector *tcpreq) exportData(dataCh chan<- model.ExportDataWarpper) {
	// logger := collector.getter.Logger()

	// 每个循环重新读取配置检查是否更新
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	conf := collector.conf

	// 导出数据
	if len(collector.tcpEventBuffer) > 0 {
		// logger.Info("tcpreq", zap.Any("buffer", collector.tcpEventBuffer))
		dataCh <- model.ExportDataWarpper{
			Data:     collector.tcpEventBuffer,
			Exporter: conf.ExportConfig.ExporterConfig,
		}
		collector.tcpEventBuffer = collector.tcpEventBuffer[:0]
	}
}

func (collector *tcpreq) onConfigChange(conf config.ColTcpreq) {
	logger := collector.getter.Logger()
	// logger.Info("fcount", zap.String("change", "before"))
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	collector.pidLis.OnConfigChange(conf.ListenConfig)
	collector.conf = conf
	logger.Info("tcpreq", zap.Any("val", "loop"))
	iter := collector.controlMap.Iter()
	for iter.Next() {
		key := iter.Key()
		val := iter.Leaf()

		// 通过指针直接操作，因为[]byte并不被看作指针，所以需要二阶解引用
		keyStruct := **(**uint32)(unsafe.Pointer(&key))
		valStruct := **(**tcpControlMap)(unsafe.Pointer(&val))
		logger.Info("tcpreq", zap.Any("val", valStruct))

		// 修改详细等级
		valStruct.Verbose = uint32(conf.Verbose)
		collector.controlMap.SetP(unsafe.Pointer(&keyStruct), unsafe.Pointer(&valStruct))
	}
	// logger.Info("fcount", zap.String("change", "after"))
}

func (collector *tcpreq) bufferEvent(e tcpEvent) {
	collector.mutex.Lock()
	defer collector.mutex.Unlock()
	conn := tcpConn{
		peerAddr: e.PeerAddr,
		myAddr:   e.MyAddr,
		peerPort: e.PeerPort,
		myPort:   e.MyPort,
	}
	conf := collector.conf
	data := &model.ExportData{
		Name: conf.ExportConfig.DataName,
		Tags: map[string]string{
			"pid":       fmt.Sprint(e.PID),
			"eventType": tcpEventTypeToString(e.Type),
			"peerAddr":  fmt.Sprintf("%s:%d", utils.UInt32ToIP(e.PeerAddr), e.PeerPort),
			"myAddr":    fmt.Sprintf("%s:%d", utils.UInt32ToIP(e.MyAddr), e.MyPort),
		},
		Fields: map[string]interface{}{
			"occurTime": time.Now(),
		},
	}
	containerName := collector.getter.GetContainerFromPID(int(e.PID))
	if containerName != "" {
		data.Tags["container"] = containerName
	}
	if pre, ok := collector.tcpEventMachine[conn]; ok {
		data.Tags["prevType"] = tcpEventTypeToString(pre.Type)
		// 从上一个状态到当前状态的间隔时间
		data.Fields["duration(ns)"] = int64(e.CurTs - pre.CurTs)
	}
	collector.tcpEventMachine[conn] = e

	collector.tcpEventBuffer = append(collector.tcpEventBuffer, data)
}

func tcpEventTypeToString(tcpType uint32) string {
	switch tcpType {
	case 1:
		return "recv"
	case 2:
		return "send"
	case 3:
		return "accept"
	case 4:
		return "close"
	}

	return "unknown"
}
