package provider

import (
	"github.com/spf13/viper"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/info"
	"go.uber.org/zap"
)

func Setup(setter *info.Setter, logger *zap.Logger) {
	configPath := viper.GetString("config")
	err := StartConfigProvider(configPath, setter, logger)
	if err != nil {
		panic(err)
	}

	err = StartContainerProvider(setter, logger)
	if err != nil {
		logger.Error("fail to get k8s client", zap.Error(err))
	}
}
