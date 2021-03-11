package info

import (
	"regexp"

	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/config"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
	"go.uber.org/zap"
)

type Getter struct {
	info      Info
	pidBuffer map[int]string
}

func NewGetter(info Info) *Getter {
	getter := &Getter{
		info:      info,
		pidBuffer: make(map[int]string),
	}

	info.events.RegisterContainerChange(func(name string, prePID, curPID []int) {
		info.mutex.Lock()
		defer info.mutex.Unlock()
		for _, pid := range prePID {
			delete(getter.pidBuffer, pid)
		}
	})

	return getter
}

func (g *Getter) Logger() *zap.Logger {
	return g.info.logger
}

func (g *Getter) GetDataCh() chan model.ExportDataWarpper {
	return g.info.dataCh
}

func (g *Getter) GetEvents() *Events {
	return &g.info.events
}

func (g *Getter) GetPIDsFromContainer(contianer string) []int {
	g.info.mutex.Lock()
	defer g.info.mutex.Unlock()

	if pids, ok := g.info.containerPID[contianer]; ok {
		ret := make([]int, len(pids))
		copy(ret, pids)
		return ret
	}
	return []int{}
}

func (g *Getter) GetContainerFromPID(pid int) string {
	g.info.mutex.Lock()
	defer g.info.mutex.Unlock()

	if name, ok := g.pidBuffer[pid]; ok {
		return name
	}

	for containerName, pids := range g.info.containerPID {
		for _, containerPID := range pids {
			if pid == containerPID {
				g.pidBuffer[pid] = containerName
				return containerName
			}
		}
	}

	g.pidBuffer[pid] = ""
	return ""
}

func (g *Getter) GetContainerPIDsMatchRegex(pattern string) []int {
	g.info.mutex.Lock()
	defer g.info.mutex.Unlock()

	ret := make([]int, 0)
	reg, err := regexp.Compile(pattern)
	if err != nil {
		return ret
	}

	for name, pids := range g.info.containerPID {
		if reg.MatchString(name) {
			g.Logger().Info("getter", zap.String("match", name))
			ret = append(ret, pids...)
		}
	}

	return ret
}

func (g *Getter) GetConfiguration() config.Config {
	return g.info.configuration
}
