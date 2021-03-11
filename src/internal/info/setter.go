package info

import (
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/config"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
	"go.uber.org/zap"
)

type Setter struct {
	info Info
}

func NewSetter(info Info) *Setter {
	return &Setter{
		info: info,
	}
}

func (s *Setter) Logger() *zap.Logger {
	return s.info.logger
}

func (s *Setter) GetDataCh() chan model.ExportDataWarpper {
	return s.info.dataCh
}

func (s *Setter) GetEvents() *Events {
	return &s.info.events
}

func (s *Setter) SetConfiguration(conf config.Config) {
	// 设置变量
	s.info.mutex.Lock()
	s.info.configuration = conf
	s.info.mutex.Unlock()

	// 通知
	for _, fn := range s.info.events.configurationChange {
		go fn()
	}
}

func (s *Setter) SetContainerPIDs(container string, pids []int) {
	// 设置变量
	s.info.mutex.Lock()
	prev, ok := s.info.containerPID[container]
	if !ok {
		prev = []int{}
	}

	if pids == nil || len(pids) == 0 {
		delete(s.info.containerPID, container)
	} else {
		s.info.containerPID[container] = pids
	}
	s.info.mutex.Unlock()

	// 通知
	cur := make([]int, len(pids))
	copy(cur, pids)
	for _, fn := range s.info.events.containerPIDChange {
		go fn(container, prev, cur)
	}
}
