package program

// import (
// 	"fmt"
// 	"unsafe"

// 	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
// 	"go.uber.org/zap"
// )

// /*
// #cgo CFLAGS: -I${SRCDIR}/../../libbpf/include/uapi -I${SRCDIR}/../../libbpf/src/build/usr/include -I${SRCDIR}/../../libbpf/src
// #cgo LDFLAGS: -L${SRCDIR}/../../libbpf/src -l:libbpf.a -lelf
// #include <bpf/bpf.h>
// #include <bpf/btf.h>
// #include <bpf/libbpf.h>
// #include <malloc.h>

// __u32 get_member_type(const struct btf_type *t, __u32 member_idx){
// 	const struct btf_member *m = btf_members(t) + member_idx;
// 	return m->type;
// }
// __u32 get_array_type(const struct btf_array *m){ return m->type; }

// extern void rawCallback(void*, void*, int);
// */
// import "C"

// // export rawCallback
// // func rawCallback(cbCookie unsafe.Pointer, raw unsafe.Pointer, rawSize C.int) {
// // 	callbackData := lookupCallback(uint64(uintptr(cbCookie)))
// // 	callbackData.receiverChan <- C.GoBytes(raw, rawSize)
// // }

// type BpfObject *C.struct_bpf_object
// type BpfMap *C.struct_bpf_map

// var ErrorCode uint64 = ^uint64(1000) + 1
// var InvaildArgumentCode = -22

// func IsErr(addr unsafe.Pointer) bool { return uintptr(addr) > uintptr(ErrorCode) }

// type BpfProgram struct {
// 	Obj  BpfObject
// 	Maps map[string]*BpfMapData
// }

// type BpfMapData struct {
// 	MapFd      int
// 	MapKeySize int64
// 	MapValSize int64
// }

// func NewBpfProgram(collector model.Collector, objPath string, attachCfg *model.EbpfAttach) (*BpfProgram, error) {
// 	program := &BpfProgram{}

// 	programCStr := C.CString(objPath)
// 	program.Obj = C.bpf_object__open(programCStr)
// 	program.Maps = make(map[string]*BpfMapData)
// 	C.free(unsafe.Pointer(programCStr))

// 	if program.Obj == nil || IsErr(unsafe.Pointer(program.Obj)) {
// 		return nil, fmt.Errorf("Fail loading file: %s", objPath)
// 	}

// 	if code, err := C.bpf_object__load(program.Obj); code != 0 || err != nil {
// 		return nil, fmt.Errorf("Fail loading object(code: %d): %w", int(code), err)
// 	}

// 	// TODO : attach more type of ebpf
// 	program.AttachKprobe(attachCfg.Kprobe, false)
// 	program.AttachKprobe(attachCfg.Kretprobe, true)

// 	return program, nil
// }

// func (prog *BpfProgram) MapGetLeaf(mapName string, key []byte) ([]byte, error) {
// 	mapInfo, err := prog.GetMapInfo(mapName)

// 	if err != nil {
// 		return nil, fmt.Errorf("get leaf error: %w", err)
// 	}

// 	value := make([]byte, mapInfo.MapValSize)

// 	keyPtr := unsafe.Pointer(&key[0])
// 	valPtr := unsafe.Pointer(&value[0])

// 	C.bpf_map_lookup_elem(C.int(mapInfo.MapFd), keyPtr, valPtr)
// 	return value, nil
// }

// func (prog *BpfProgram) MapSetLeaf(mapName string, key []byte, val []byte) error {
// 	mapInfo, err := prog.GetMapInfo(mapName)
// 	if err != nil {
// 		return err
// 	}

// 	keyPtr := unsafe.Pointer(&key[0])
// 	valPtr := unsafe.Pointer(&val[0])

// 	code, err := C.bpf_map_update_elem(C.int(mapInfo.MapFd), keyPtr, valPtr, C.ulonglong(0))
// 	if err != nil {
// 		return err
// 	}
// 	if code != 0 {
// 		return fmt.Errorf("Error, code: %d", code)
// 	}

// 	return nil
// }

// func (prog *BpfProgram) MapDeleteLeaf(mapName string, key []byte) error {
// 	mapInfo, err := prog.GetMapInfo(mapName)
// 	if err != nil {
// 		return err
// 	}

// 	keyPtr := unsafe.Pointer(&key[0])

// 	code, err := C.bpf_map_delete_elem(C.int(mapInfo.MapFd), keyPtr)
// 	if err != nil {
// 		return err
// 	}
// 	if code != 0 {
// 		return fmt.Errorf("Error, code: %d", code)
// 	}

// 	return nil
// }

// func (prog *BpfProgram) MapForEach(mapName string, action func(key, val []byte) bool) error {
// 	mapInfo, err := prog.GetMapInfo(mapName)
// 	if err != nil {
// 		return err
// 	}

// 	curKey := make([]byte, mapInfo.MapKeySize)
// 	nextKey := make([]byte, mapInfo.MapKeySize)

// 	keyPtr := unsafe.Pointer(&curKey[0])
// 	nextKeyPtr := unsafe.Pointer(&nextKey[0])

// 	for {
// 		code, err := C.bpf_map_get_next_key(C.int(mapInfo.MapFd), keyPtr, nextKeyPtr)
// 		if err != nil {
// 			return err
// 		}

// 		if code == 0 {
// 			copy(curKey, nextKey)
// 			value, err := prog.MapGetLeaf(mapName, curKey)
// 			if err != nil {
// 				return err
// 			}
// 			// logger.Info("curKey", zap.String("key", fmt.Sprint(curKey)), zap.String("val", fmt.Sprint(value)))
// 			if !action(curKey, value) {
// 				return nil
// 			}
// 		} else {
// 			if code != -1 {
// 				return fmt.Errorf("lookup map fail, code:%d", code)
// 			}
// 			break
// 		}
// 	}

// 	return nil
// }

// func (prog *BpfProgram) PerfEventOutput(mapName string, ch *chan []byte) error {
// 	// TODO: 未完成

// 	// // 获取map结构体
// 	// bpfMap, err := getMap(prog.Obj, mapName)
// 	// if err != nil {
// 	// 	return fmt.Errorf("cannot get bpfMap[%s]: %w", mapName, err)
// 	// }

// 	// // 获取map文件描述符
// 	// mapFd, err := getMapFd(bpfMap)
// 	// if err != nil {
// 	// 	return fmt.Errorf("cannot get bpfMapFd[%s]: %w", mapName, err)
// 	// }

// 	return fmt.Errorf("Not implement yet")
// }

// func (prog *BpfProgram) Close() {
// }

// func (prog *BpfProgram) GetMapInfo(mapName string) (*BpfMapData, error) {
// 	if ebpfMap, ok := prog.Maps[mapName]; ok {
// 		return ebpfMap, nil
// 	}

// 	// 获取map结构体
// 	bpfMap, err := getMap(prog.Obj, mapName)
// 	if err != nil {
// 		return nil, fmt.Errorf("cannot get bpfMap[%s]: %w", mapName, err)
// 	}

// 	// 获取map文件描述符
// 	mapFd, err := getMapFd(bpfMap)
// 	if err != nil {
// 		return nil, fmt.Errorf("cannot get bpfMapFd[%s]: %w", mapName, err)
// 	}

// 	// 获取结构体内key和value的id
// 	bpfBtf, err := C.bpf_object__btf(prog.Obj)
// 	if bpfBtf == nil {
// 		return nil, fmt.Errorf("Error, missing btf, are you complier with btf?")
// 	}

// 	keyTypeID := C.bpf_map__btf_key_type_id(bpfMap)
// 	valTypeID := C.bpf_map__btf_value_type_id(bpfMap)

// 	prog.Maps[mapName] = &BpfMapData{
// 		MapFd:      mapFd,
// 		MapKeySize: int64(C.btf__resolve_size(bpfBtf, keyTypeID)),
// 		MapValSize: int64(C.btf__resolve_size(bpfBtf, valTypeID)),
// 	}

// 	return prog.Maps[mapName], nil
// }

// func (prog *BpfProgram) AttachKprobe(attachMap map[string]string, ret bool) {
// 	obj := prog.Obj
// 	for funcName, attachName := range attachMap {
// 		logger.Info("Attach kprobe", zap.String("type", attachName), zap.String("attach", funcName))

// 		progsec := C.CString(funcName)
// 		probe := C.CString(attachName)
// 		if prog, err := C.bpf_object__find_program_by_title(obj, progsec); prog != nil && err != nil {
// 			link, err := C.bpf_program__attach_kprobe(prog, C._Bool(ret), probe)

// 			if link == nil || err != nil {
// 				logger.Error("Fail attach probe", zap.String("attachName", attachName), zap.String("functionName", funcName), zap.Error(err))
// 			}
// 		} else {
// 			logger.Error("Fail finding progsec", zap.String("attachName", attachName), zap.String("functionName", funcName), zap.Error(err))
// 		}
// 		C.free(unsafe.Pointer(probe))
// 		C.free(unsafe.Pointer(progsec))
// 	}
// }

// func (prog *BpfProgram) SocketAttach(funcName string, itfName string) (int, error) {
// 	return -1, fmt.Errorf("Not implement yet")
// }

// func (prog *BpfProgram) PerfEventAttach(funcName string, evType, evConfig int, samplePeriod int, sampleFreq int, pid, cpu, groupFd int) error {
// 	return fmt.Errorf("Not implement yet")
// }

// func getMap(obj BpfObject, mapName string) (BpfMap, error) {
// 	mapNameCStr := C.CString(mapName)
// 	bpfMap, err := C.bpf_object__find_map_by_name(obj, mapNameCStr)
// 	C.free(unsafe.Pointer(mapNameCStr))

// 	return bpfMap, err
// }

// func getMapFd(bpfMap BpfMap) (int, error) {
// 	mapFd, err := C.bpf_map__fd(bpfMap)
// 	if err != nil {
// 		return 0, err
// 	}

// 	goMapFd := int(mapFd)

// 	if goMapFd < 0 {
// 		if goMapFd == InvaildArgumentCode {
// 			return 0, fmt.Errorf("Argument Invalid")
// 		}

// 		return 0, fmt.Errorf("Fail to get Fd of map")
// 	}

// 	return goMapFd, nil
// }

// // // TODO: Callback 接口
// // func PerfCallback() {

// // }

// // var callbackRegister = make(map[uint64]*callbackData)
// // var callbackIndex uint64
// // var mu sync.Mutex

// // type callbackData struct {
// // 	receiverChan chan []byte
// // 	lostChan     chan uint64
// // }

// // func lookupCallback(i uint64) *callbackData {
// // 	return callbackRegister[i]
// // }
