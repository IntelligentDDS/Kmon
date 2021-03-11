package provider

import (
	"github.com/fsnotify/fsnotify"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/config"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/info"
	"go.uber.org/zap"
)

type ConfigProvider struct {
	watcher *fsnotify.Watcher
	doneCh  chan bool
}

var proConfig ConfigProvider

func StartConfigProvider(configPath string, setter *info.Setter, logger *zap.Logger) error {
	// 相关变量初始化
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		panic(err)
	}

	proConfig = ConfigProvider{
		watcher: watcher,
		doneCh:  make(chan bool),
	}

	go func() {
		for {
			select {
			case event := <-proConfig.watcher.Events:
				logger.Info("config", zap.Any("event", event))
				if event.Op&fsnotify.Write == fsnotify.Write {
					conf, err := config.FindConfig(configPath)
					if err == nil {
						setter.SetConfiguration(conf)
					}
				}
			case err := <-proConfig.watcher.Errors:
				logger.Error("config", zap.Error(err))
			case <-proConfig.doneCh:
				return
			}
		}
	}()

	err = proConfig.watcher.Add(configPath)
	if err != nil {
		return err
	}

	setter.GetEvents().RegisterProgramEnd(func() {
		proConfig.doneCh <- true
		proConfig.watcher.Close()
	})

	return nil
}
