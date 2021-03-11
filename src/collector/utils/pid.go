package utils

import (
	"regexp"
	"sync"

	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/config"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/info"
	"go.uber.org/zap"
)

type PIDLis struct {
	conf         config.ListenConfig
	getter       *info.Getter
	mutex        *sync.Mutex
	containerSet map[int]bool
	pidSet       map[int]bool
	AddCh        chan int
	DelCh        chan int
}

func NewPIDLis(getter *info.Getter, conf config.ListenConfig, bufferSize int, logger *zap.Logger) *PIDLis {
	c := &PIDLis{
		conf:   conf,
		getter: getter,
		mutex:  &sync.Mutex{},
		AddCh:  make(chan int, bufferSize),
		DelCh:  make(chan int, bufferSize),
	}

	getter.GetEvents().RegisterContainerChange(func(name string, prePID, curPID []int) {
		logger.Info("pidlis", zap.String("name", name), zap.Any("pre", prePID), zap.Any("cur", curPID))
		for _, exp := range c.conf.K8sGroup {
			// 判断变动与该项是否有关
			match, err := regexp.MatchString(exp, name)
			if err != nil || !match {
				continue
			}

			// 比较得到增删的PID
			delta := map[int]bool{}
			for _, pid := range prePID {
				delete(c.containerSet, pid)
			}
			for _, pid := range curPID {
				delta[pid] = true
			}

			// 根据状态确认是否需要修改
			c.mutex.Lock()
			for pid, state := range delta {
				if state {
					if _, ok := c.containerSet[pid]; !ok {
						c.containerSet[pid] = true
					} else {
						delete(delta, pid)
					}
				} else {
					delta[pid] = false
				}
			}
			c.mutex.Unlock()

			// 通知状态更改
			c.notifyChange(delta)
		}
	})

	return c
}

func (c *PIDLis) OnConfigChange(conf config.ListenConfig) {
	logger := c.getter.Logger()
	delta := map[int]bool{}

	c.mutex.Lock()
	defer c.notifyChange(delta) // 确保通知在锁释放之后发出
	defer c.mutex.Unlock()

	for pid := range c.containerSet {
		delta[pid] = false
	}
	for pid := range c.pidSet {
		delta[pid] = false
	}

	// 重新获取固定PID
	c.pidSet = make(map[int]bool)
	for _, pid := range conf.PIDGroup {
		delta[pid] = true
		c.pidSet[pid] = true
	}

	// 重新获取动态PID
	c.containerSet = make(map[int]bool)
	for _, re := range conf.K8sGroup {
		for _, pid := range c.getter.GetContainerPIDsMatchRegex(re) {
			logger.Info("pidLis", zap.Int("pid", pid))
			delta[pid] = true
			c.containerSet[pid] = true
		}
	}

}

func (c *PIDLis) notifyChange(delta map[int]bool) {
	// TODO: 防止阻塞使用了goroutine，但未确保修改先后顺序
	// 错误的先后顺序会导致通知发生问题，或许需要消息队列？
	go func() {
		for pid, state := range delta {
			if state {
				c.AddCh <- pid
			} else {
				c.DelCh <- pid
			}
		}
	}()
}
