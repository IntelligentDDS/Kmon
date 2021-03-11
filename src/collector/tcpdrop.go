package collector

import (
	"fmt"
	"net"
	"strings"
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

type tcpdrop struct {
	mutex           sync.Mutex
	conf            config.ColTcpdrop
	module          *bcc.Module
	controlMap      *bcc.Table
	stackMap        *bcc.Table
	dropV4Ch        chan []byte
	dropV6Ch        chan []byte
	dropLostCh      chan uint64
	dropEventBuffer []*model.ExportData
	getter          *info.Getter
	pidLis          *utils.PIDLis
}

type dropPackV4 struct {
	PID         uint32
	StackID     uint32
	UserStackID uint32
	State       uint8
	TCPflags    uint8
	SPort       uint16
	DPort       uint16
	SAddr       uint32
	DAddr       uint32
}

// TODO: check
type dropPackV6 struct {
	PID         uint32
	StackID     uint32
	UserStackID uint32
	State       uint8
	TCPflags    uint8
	SPort       uint16
	DPort       uint16
	SAddr       [16]byte
	DAddr       [16]byte
}

type tcpdropControlMap struct {
	Verbose uint32
}

// StartTcpdrop ...
func StartTcpdrop(confCh <-chan config.ColTcpdrop, getter *info.Getter) error {
	logger := getter.Logger()

	// 初始化
	conf := <-confCh
	collector, err := setupTcpdropBCC(conf, getter)
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
			case l := <-collector.dropLostCh:
				logger.Warn("tcpdrop", zap.Uint64("lost_event", l))
			case ev4 := <-collector.dropV4Ch:
				val := **(**dropPackV4)(unsafe.Pointer(&ev4))
				collector.bufferEventV4(val)
			case ev6 := <-collector.dropV6Ch:
				val := **(**dropPackV6)(unsafe.Pointer(&ev6))
				collector.bufferEventV6(val)
			case addPID := <-collector.pidLis.AddCh:
				collector.mutex.Lock()
				con := bioControlMap{
					Verbose: collector.conf.Verbose,
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

func setupTcpdropBCC(conf config.ColTcpdrop, getter *info.Getter) (*tcpdrop, error) {
	logger := getter.Logger()
	module, err := loadModule("./bcc/tcpdrop.c", logger)
	if err != nil {
		return nil, err
	}

	err = attachKprobe(module, "trace_tcp_drop", "tcp_drop", false)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}
	dropLostCh := make(chan uint64)

	dropV4Ch := make(chan []byte)
	dropV4Table := bcc.NewTable(module.TableId("ipv4_events"), module)
	perfV4Map, err := bcc.InitPerfMap(dropV4Table, dropV4Ch, dropLostCh)
	if err != nil {
		return nil, err
	}
	perfV4Map.Start()

	dropV6Ch := make(chan []byte)
	dropV6Table := bcc.NewTable(module.TableId("ipv6_events"), module)
	perfV6Map, err := bcc.InitPerfMap(dropV6Table, dropV6Ch, dropLostCh)
	if err != nil {
		return nil, err
	}
	perfV6Map.Start()

	// 不要在这里赋值conf，初始化时需要执行onConfigChange来进行PID的设置
	return &tcpdrop{
		mutex:      sync.Mutex{},
		module:     module,
		conf:       config.ColTcpdrop{},
		pidLis:     utils.NewPIDLis(getter, config.ListenConfig{}, 1024, logger),
		controlMap: bcc.NewTable(module.TableId("control_map"), module),
		stackMap:   bcc.NewTable(module.TableId("stack_traces"), module),
		dropV4Ch:   dropV4Ch,
		dropV6Ch:   dropV6Ch,
		dropLostCh: dropLostCh,
		getter:     getter,
	}, nil
}

func (collector *tcpdrop) bufferEventV4(e dropPackV4) {
	// logger := collector.getter.Logger()
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	stateStr, ok := tcpState[e.State]
	if !ok {
		stateStr = "UNKNOWN"
	}

	// 记录事件
	conf := collector.conf
	data := &model.ExportData{
		Name: conf.ExportConfig.DataName,
		Tags: map[string]string{
			"pid":         fmt.Sprint(e.PID),
			"source":      fmt.Sprintf("%s:%d", utils.UInt32ToIP(e.SAddr), e.SPort),
			"destination": fmt.Sprintf("%s:%d", utils.UInt32ToIP(e.DAddr), e.DPort),
		},
		Fields: map[string]interface{}{
			"flag":      flag2str(e.TCPflags),
			"state":     stateStr,
			"timestamp": time.Now(),
		},
	}

	// 分析栈
	if e.StackID != 0 {
		if stackStr, err := exportStack(collector.stackMap, e.StackID, true); err == nil {
			data.Fields["kernelStack"] = stackStr
		}
	}
	if e.UserStackID != 0 {
		if stackStr, err := exportStack(collector.stackMap, e.StackID, false); err == nil {
			data.Fields["userStack"] = stackStr
		}
	}

	containerName := collector.getter.GetContainerFromPID(int(e.PID))
	if containerName != "" {
		data.Tags["container"] = containerName
	}

	collector.dropEventBuffer = append(collector.dropEventBuffer, data)
}

func (collector *tcpdrop) bufferEventV6(e dropPackV6) {
	// logger := collector.getter.Logger()
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	stateStr, ok := tcpState[e.State]
	if !ok {
		stateStr = "UNKNOWN"
	}

	// 记录事件
	conf := collector.conf
	data := &model.ExportData{
		Name: conf.ExportConfig.DataName,
		Tags: map[string]string{
			"pid":         fmt.Sprint(e.PID),
			"source":      fmt.Sprintf("%s:%d", net.IP(e.SAddr[:]), e.SPort),
			"destination": fmt.Sprintf("%s:%d", net.IP(e.DAddr[:]), e.SPort),
		},
		Fields: map[string]interface{}{
			"flag":      flag2str(e.TCPflags),
			"state":     stateStr,
			"timestamp": time.Now(),
		},
	}

	// 分析栈
	if e.StackID != 0 {
		if stackStr, err := exportStack(collector.stackMap, e.StackID, true); err == nil {
			data.Fields["kernelStack"] = stackStr
		}
	}
	if e.UserStackID != 0 {
		if stackStr, err := exportStack(collector.stackMap, e.StackID, false); err == nil {
			data.Fields["userStack"] = stackStr
		}
	}

	containerName := collector.getter.GetContainerFromPID(int(e.PID))
	if containerName != "" {
		data.Tags["container"] = containerName
	}

	collector.dropEventBuffer = append(collector.dropEventBuffer, data)
}

func (collector *tcpdrop) exportData(dataCh chan<- model.ExportDataWarpper) {
	// logger := collector.getter.Logger()

	// 每个循环重新读取配置检查是否更新
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	conf := collector.conf

	// 导出数据
	if len(collector.dropEventBuffer) > 0 {
		// logger.Info("tcpdrop", zap.Any("data", collector.dropEventBuffer))
		dataCh <- model.ExportDataWarpper{
			Data:     collector.dropEventBuffer,
			Exporter: conf.ExportConfig.ExporterConfig,
		}
		collector.dropEventBuffer = collector.dropEventBuffer[:0]
	}
}

func (collector *tcpdrop) onConfigChange(conf config.ColTcpdrop) {
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
		valStruct := **(**tcpdropControlMap)(unsafe.Pointer(&val))
		logger.Info("Tcpdrop", zap.Any("val", valStruct))

		// 修改详细等级
		valStruct.Verbose = uint32(conf.Verbose)
		collector.controlMap.SetP(unsafe.Pointer(&keyStruct), unsafe.Pointer(&valStruct))
	}
	// logger.Info("fcount", zap.String("change", "after"))
}

const (
	TCPHeaderFIN = 0x01
	TCPHeaderSYN = 0x02
	TCPHeaderRST = 0x04
	TCPHeaderPSH = 0x08
	TCPHeaderACK = 0x10
	TCPHeaderURG = 0x20
	TCPHeaderECE = 0x40
	TCPHeaderCWR = 0x80
)

func flag2str(flags uint8) string {
	var fstrs []string

	if flags&TCPHeaderFIN != 0 {
		fstrs = append(fstrs, "FIN")
	}
	if flags&TCPHeaderSYN != 0 {
		fstrs = append(fstrs, "SYN")
	}
	if flags&TCPHeaderRST != 0 {
		fstrs = append(fstrs, "RST")
	}
	if flags&TCPHeaderPSH != 0 {
		fstrs = append(fstrs, "PSH")
	}
	if flags&TCPHeaderURG != 0 {
		fstrs = append(fstrs, "URG")
	}
	if flags&TCPHeaderECE != 0 {
		fstrs = append(fstrs, "ECE")
	}
	if flags&TCPHeaderCWR != 0 {
		fstrs = append(fstrs, "CWR")
	}

	return strings.Join(fstrs, "|")
}

var tcpState map[uint8]string = map[uint8]string{
	1:  "ESTABLISHED",
	2:  "SYN_SENT",
	3:  "SYN_RECV",
	4:  "FIN_WAIT1",
	5:  "FIN_WAIT2",
	6:  "TIME_WAIT",
	7:  "CLOSE",
	8:  "CLOSE_WAIT",
	9:  "LAST_ACK",
	10: "LISTEN",
	11: "CLOSING",
	12: "NEW_SYN_RECV",
}
