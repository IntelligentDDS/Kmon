package cmd

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/collector"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/exporter"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/config"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/info"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/provider"
	"go.uber.org/zap"
)

var logger, _ = zap.NewDevelopment()
var exportFunc map[string]model.ExportFunc

var rootCmd = &cobra.Command{
	Use:   "kmon",
	Short: "kmon is a microservice monitor tools using ebpf",
	Long:  `kmon is a microservice monitor tools using ebpf`,
	Run: func(cmd *cobra.Command, args []string) {
		configPath := viper.GetString("config")
		programStart(configPath)
	},
}

func Execute() {
	// Parameters config
	pflag.StringP("config", "c", "./config.yaml", "path of running config")
	viper.BindPFlags(pflag.CommandLine)

	// Environment config
	viper.BindEnv("config")

	if err := rootCmd.Execute(); err != nil {
		logger.Error("Execute root command fail", zap.Error(err))
		os.Exit(1)
	}
}

func programStart(configPath string) {
	infoData := info.NewInfo(logger)
	getter, setter := info.NewGetter(infoData), info.NewSetter(infoData)

	// 获取配置
	conf, err := config.FindConfig(configPath)
	if err != nil {
		panic(err)
	}
	logger.Info("set conf")
	setter.SetConfiguration(conf)

	// 初始化并运行部件
	provider.Setup(setter, logger)
	collector.Setup(getter, logger)
	exporter.Setup(getter, logger)

	// 等待程序结束
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan
	logger.Info("program is terminating...")
	getter.GetEvents().EndProgram()
	logger.Info("done, bye :)")
}
