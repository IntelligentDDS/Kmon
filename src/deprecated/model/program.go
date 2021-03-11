package model

type Program interface {
	MapForEach(mapName string, action func(key, val []byte) bool) error
	MapGetLeaf(mapName string, key []byte) ([]byte, error)
	MapSetLeaf(mapName string, key []byte, val []byte) error
	MapDeleteLeaf(mapName string, key []byte) error

	PerfEventAttach(funcName string, evType, evConfig int, samplePeriod int, sampleFreq int, pid, cpu, groupFd int) error
	SocketAttach(funcName string, itfName string) (int, error)

	PerfEventOutput(mapName string, ch *chan []byte) error

	Close()
}

// type PerfOutputData struct {
// 	From     ProgramConfig
// 	Name     string
// 	DataType *MapType
// 	Data     *[]byte
// }
