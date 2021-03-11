package collector

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/iovisor/gobpf/bcc"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/collector/utils"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/config"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/info"
	"go.uber.org/zap"
)

type ColFcountManager struct {
	ConfCh chan config.ColFcount
}

type ColTcpreqManager struct {
	ConfCh chan config.ColTcpreq
}

type ColBiostatManager struct {
	ConfCh chan config.ColBiostat
}

type ColTcpdropManager struct {
	ConfCh chan config.ColTcpdrop
}

type ColOffcpuManager struct {
	ConfCh chan config.ColOffcpu
}

type CollectorManager struct {
	ColFcount  map[string]*ColFcountManager
	ColTcpreq  map[string]*ColTcpreqManager
	ColBiostat map[string]*ColBiostatManager
	ColTcpdrop map[string]*ColTcpdropManager
	ColOffcpu  map[string]*ColOffcpuManager
}

var Manager = CollectorManager{}
var colMutex = sync.Mutex{}

// Setup ...
func Setup(getter *info.Getter, logger *zap.Logger) {
	totalConfig := getter.GetConfiguration().Collector
	{
		Manager.ColFcount = make(map[string]*ColFcountManager)
		for _, conf := range totalConfig.Fcount {
			confCh := make(chan config.ColFcount, 1)
			confCh <- conf
			err := StartFcount(confCh, getter)

			if err != nil {
				logger.Error("", zap.Error(err))
				continue
			}

			Manager.ColFcount[conf.Name] = &ColFcountManager{
				ConfCh: confCh,
			}
		}
	}

	{
		Manager.ColTcpreq = make(map[string]*ColTcpreqManager)
		for _, conf := range totalConfig.Tcpreq {
			confCh := make(chan config.ColTcpreq, 1)
			confCh <- conf

			err := StartTcpreq(confCh, getter)

			if err != nil {
				logger.Error("", zap.Error(err))
				continue
			}

			Manager.ColTcpreq[conf.Name] = &ColTcpreqManager{
				ConfCh: confCh,
			}

		}
	}

	{
		Manager.ColBiostat = make(map[string]*ColBiostatManager)
		for _, conf := range totalConfig.Biostat {
			confCh := make(chan config.ColBiostat, 1)
			confCh <- conf
			err := StartBiostat(confCh, getter)

			if err != nil {
				logger.Error("", zap.Error(err))
				continue
			}

			Manager.ColBiostat[conf.Name] = &ColBiostatManager{
				ConfCh: confCh,
			}
		}

		{
			Manager.ColTcpdrop = make(map[string]*ColTcpdropManager)
			for _, conf := range totalConfig.Tcpdrop {
				confCh := make(chan config.ColTcpdrop, 1)
				confCh <- conf
				err := StartTcpdrop(confCh, getter)

				if err != nil {
					logger.Error("", zap.Error(err))
					continue
				}

				Manager.ColTcpdrop[conf.Name] = &ColTcpdropManager{
					ConfCh: confCh,
				}
			}
		}

		{
			Manager.ColOffcpu = make(map[string]*ColOffcpuManager)
			for _, conf := range totalConfig.Offcpu {
				confCh := make(chan config.ColOffcpu, 1)
				confCh <- conf
				err := StartOffcpu(confCh, getter)

				if err != nil {
					logger.Error("", zap.Error(err))
					continue
				}

				Manager.ColOffcpu[conf.Name] = &ColOffcpuManager{
					ConfCh: confCh,
				}
			}
		}
	}

	// 将配置变更通知给所有的Collector
	getter.GetEvents().RegisterConfigurationChange(func() {
		colMutex.Lock()
		defer colMutex.Unlock()

		conf := getter.GetConfiguration()
		for _, colConf := range conf.Collector.Fcount {
			localConf := colConf
			go func() {
				Manager.ColFcount[localConf.Name].ConfCh <- localConf
			}()
		}

		for _, colConf := range conf.Collector.Tcpreq {
			localConf := colConf
			go func() {
				Manager.ColTcpreq[localConf.Name].ConfCh <- localConf
			}()
		}

		for _, colConf := range conf.Collector.Biostat {
			localConf := colConf
			go func() {
				Manager.ColBiostat[localConf.Name].ConfCh <- localConf
			}()
		}

		for _, colConf := range conf.Collector.Tcpdrop {
			localConf := colConf
			go func() {
				Manager.ColTcpdrop[localConf.Name].ConfCh <- localConf
			}()
		}

		for _, colConf := range conf.Collector.Offcpu {
			localConf := colConf
			go func() {
				Manager.ColOffcpu[localConf.Name].ConfCh <- localConf
			}()
		}
	})
}

func attachKprobe(module *bcc.Module, kprobeName, funcName string, isRet bool) error {
	kprobe, err := module.LoadKprobe(kprobeName)
	if err != nil {
		return err
	}

	if isRet {
		err = module.AttachKretprobe(funcName, kprobe, -1)
	} else {
		err = module.AttachKprobe(funcName, kprobe, -1)
	}

	if err != nil {
		return err
	}

	return nil
}

func loadModule(path string, logger *zap.Logger) (*bcc.Module, error) {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Error("", zap.Error(err))
		return nil, err
	}

	sourceCode := string(file)
	module := bcc.NewModule(sourceCode, []string{})
	if module == nil {
		logger.Error("", zap.Error(err))
		return nil, fmt.Errorf("Fail to load module")
	}

	return module, nil
}

func waitPeriod(period int, done <-chan bool) bool {
	interval := 1000
	passTime := 0
	for ; passTime < period-interval; passTime += interval {
		select {
		case <-done:
			return true
		default:
			time.Sleep(time.Duration(interval) * time.Millisecond)
		}
	}

	select {
	case <-done:
		return true
	default:
		time.Sleep(time.Duration(period-passTime) * time.Millisecond)
	}
	return false
}

func exportStack(table *bcc.Table, stackID uint32, kernel bool) (string, error) {
	id, err := utils.ToByte(stackID)
	if err != nil {
		return "", err
	}

	stackRaw, err := table.Get(id)
	if err != nil {
		return "", err
	}

	stack := make([]uint64, 127)
	if err := binary.Read(bytes.NewBuffer(stackRaw), utils.GetHostEndian(), &stack); err != nil {
		return "", err
	}

	stackStr := make([]string, 0)
	for _, stackPtr := range stack {
		if stackPtr == 0 {
			break
		}

		var funcName string
		if kernel {
			funcName = utils.Ksym(stackPtr)
			funcName = fmt.Sprintf("%s(0x%016x)", funcName, stackPtr)
		} else {
			funcName = fmt.Sprintf("0x%016x", stackPtr)
		}

		stackStr = append(stackStr, funcName)
	}

	return strings.Join(stackStr, "\n"), nil
}
