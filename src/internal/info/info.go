package info

import (
	"sync"

	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/config"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
	"go.uber.org/zap"
)

var dataBufferSize = 2048

type Info *info

type info struct {
	mutex         sync.Mutex
	events        Events
	dataCh        chan model.ExportDataWarpper
	containerPID  map[string][]int
	configuration config.Config
	logger        *zap.Logger
}

type Events struct {
	programEnd          []func()
	configurationChange []func()
	containerPIDChange  []func(name string, prePID, curPID []int)
}

func NewInfo(logger *zap.Logger) Info {
	return &info{
		mutex:  sync.Mutex{},
		dataCh: make(chan model.ExportDataWarpper, dataBufferSize),
		events: Events{
			programEnd:          make([]func(), 0),
			containerPIDChange:  make([]func(name string, prePID, curPID []int), 0),
			configurationChange: make([]func(), 0),
		},
		containerPID: make(map[string][]int),
		logger:       logger,
	}
}

func (e *Events) EndProgram() {
	// 阻塞式
	for _, fn := range e.programEnd {
		fn()
	}
}

func (e *Events) RegisterProgramEnd(fn func()) {
	e.programEnd = append(e.programEnd, fn)
}

func (e *Events) RegisterContainerChange(fn func(name string, prePID, curPID []int)) {
	e.containerPIDChange = append(e.containerPIDChange, fn)
}

func (e *Events) RegisterConfigurationChange(fn func()) {
	e.configurationChange = append(e.configurationChange, fn)
}
