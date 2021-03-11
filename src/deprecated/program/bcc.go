package program

// import (
// 	"fmt"
// 	"net"
// 	"syscall"

// 	"github.com/iovisor/gobpf/bcc"
// 	"gitlab.dds-sysu.tech/Wuny/kmon/src/utils"
// 	"go.uber.org/zap"
// )

// type BccProgram struct {
// 	Module  *bcc.Module
// 	perfMap []*bcc.PerfMap
// 	Table   map[string]*bcc.Table
// }

// func NewBccProgram(sourceCode string) (*BccProgram, error) {
// 	program := &BccProgram{
// 		Table: make(map[string]*bcc.Table),
// 	}
// 	program.Module = bcc.NewModule(sourceCode, []string{})
// 	if program.Module == nil {
// 		return nil, fmt.Errorf("Fail to load module")
// 	}

// 	return program, nil
// }

// func (prog *BccProgram) getTable(tableName string) *bcc.Table {
// 	table, ok := prog.Table[tableName]
// 	if !ok {
// 		table = bcc.NewTable(prog.Module.TableId(tableName), prog.Module)
// 		prog.Table[tableName] = table
// 	}

// 	return table
// }

// func (prog *BccProgram) MapForEach(mapName string, action func(key, val []byte) bool) error {
// 	table := prog.getTable(mapName)

// 	iter := table.Iter()
// 	for iter.Next() {
// 		key := iter.Key()
// 		val := iter.Leaf()
// 		if !action(key, val) {
// 			return nil
// 		}
// 	}

// 	return nil
// }

// func (prog *BccProgram) MapGetLeaf(mapName string, key []byte) ([]byte, error) {
// 	table := prog.getTable(mapName)
// 	return table.Get(key)
// }

// func (prog *BccProgram) MapSetLeaf(mapName string, key []byte, val []byte) error {
// 	table := prog.getTable(mapName)
// 	return table.Set(key, val)
// }

// func (prog *BccProgram) MapDeleteLeaf(mapName string, key []byte) error {
// 	table := prog.getTable(mapName)
// 	return table.Delete(key)
// }

// func (prog *BccProgram) PerfEventOutput(mapName string, ch *chan []byte) error {
// 	table := prog.getTable(mapName)

// 	perfMap, err := bcc.InitPerfMap(table, *ch, nil)
// 	if err != nil {
// 		return err
// 	}

// 	prog.perfMap = append(prog.perfMap, perfMap)
// 	perfMap.Start()

// 	return nil
// }

// func (prog *BccProgram) Close() {
// 	for _, perfMpa := range prog.perfMap {
// 		perfMpa.Stop()
// 	}
// 	prog.Module.Close()
// }

// func (prog *BccProgram) AttachKprobe(attachName string, funcName string, ret bool) error {
// 	kprobe, err := prog.Module.LoadKprobe(funcName)
// 	if err != nil {
// 		return err
// 	}

// 	if ret {
// 		err = prog.Module.AttachKretprobe(attachName, kprobe, -1)
// 	} else {
// 		err = prog.Module.AttachKprobe(attachName, kprobe, -1)
// 	}

// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// func (prog *BccProgram) SocketAttach(funcName string, itfName string) (int, error) {
// 	// 获取函数
// 	const BPF_SOCKET = 1
// 	funcFd, err := prog.Module.Load(funcName, BPF_SOCKET, 0, 0)
// 	if err != nil {
// 		return -1, fmt.Errorf("load function error: %w", err)
// 	}

// 	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, int(utils.Htons(0x800)))
// 	if err != nil {
// 		return -1, fmt.Errorf("create socket error: %w", err)
// 	}

// 	itf, err := net.InterfaceByName(itfName)
// 	if err != nil {
// 		return -1, fmt.Errorf("get interface error: %w", err)
// 	}

// 	syscall.Bind(fd, &syscall.SockaddrLinklayer{
// 		Ifindex:  itf.Index,
// 		Protocol: utils.Htons(syscall.ETH_P_ALL),
// 	})

// 	// // 架构相关，这里是从linux源码定义里拿出来的
// 	const SO_ATTACH_BPF = 50
// 	syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, SO_ATTACH_BPF, funcFd)

// 	return fd, nil
// }

// func (prog *BccProgram) PerfEventAttach(funcName string, evType, evConfig int, samplePeriod int, sampleFreq int, pid, cpu, groupFd int) error {
// 	logger.Info("Load Perf",
// 		zap.String("attach", funcName),
// 		zap.Int("evType", evType),
// 		zap.Int("evConfig", evConfig),
// 	)

// 	perfFd, err := prog.Module.LoadPerfEvent(funcName)
// 	if err != nil {
// 		return fmt.Errorf("fail load perf event: %w", err)
// 	}

// 	err = prog.Module.AttachPerfEvent(evType, evConfig, samplePeriod, sampleFreq, pid, cpu, groupFd, perfFd)
// 	if err != nil {
// 		return fmt.Errorf("fail to attach perf event: %w", err)
// 	}

// 	return nil
// }
