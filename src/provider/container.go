package provider

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	kw_config "github.com/bitnami-labs/kubewatch/config"
	kw_controller "github.com/bitnami-labs/kubewatch/pkg/controller"
	kw_event "github.com/bitnami-labs/kubewatch/pkg/event"
	"github.com/sirupsen/logrus"
	"gitlab.dds-sysu.tech/Wuny/kmon/src/internal/info"
	"go.uber.org/zap"
	api_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type ContainerProvider struct {
	handler *myHandler
	logger  *zap.Logger
}

var proContainer ContainerProvider

type myHandler struct {
	setter *info.Setter
}

func init() {
	logrus.SetLevel(logrus.PanicLevel)
}

// ObjectCreated sends events on object creation
func (h *myHandler) ObjectCreated(obj interface{}) {
	pod, ok := obj.(*api_v1.Pod)
	if ok {
		// 创建Pod的时候还没办法拿到container的ID信息，因此这里只是将pod的信息存起来
		name := fmt.Sprintf("%s/%s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
		// h.pids[name] = make([]int, 0)
		proContainer.logger.Info("create", zap.String("pod", name))
	}
}

// ObjectDeleted sends events on object deletion
func (h *myHandler) ObjectDeleted(obj interface{}) {
	deleteEvent, ok := obj.(kw_event.Event)
	if ok {
		h.setter.SetContainerPIDs(deleteEvent.Name, nil)
		proContainer.logger.Info("delete", zap.String("pod", deleteEvent.Name))
	}
}

// ObjectUpdated sends events on object updation
func (h *myHandler) ObjectUpdated(oldObj, newObj interface{}) {
	logger := h.setter.Logger()
	pids := make([]int, 0)

	pod, ok := oldObj.(*api_v1.Pod)
	if !ok {
		// 不是Pod变更，无视
		return
	}

	// 遍历当前Pod内的所有container
	for _, container := range pod.Status.ContainerStatuses {
		if len(container.ContainerID) <= 9 {
			// 无效ID，container尚未完全生成
			continue
		}

		// 除去前面的"docker://"前缀
		pid, err := getPodPID(pod, container.ContainerID[9:])
		if err != nil || pid == -1 {
			// container对应ID不处于本机
			continue
		}

		// 有效的本机ID
		pids = append(pids, pid)
	}

	if updateEvent, ok := newObj.(kw_event.Event); ok {
		logger.Info("container", zap.String("name", updateEvent.Name))
		h.setter.SetContainerPIDs(updateEvent.Name, pids)
		proContainer.logger.Info("update", zap.String("pod", updateEvent.Name), zap.Any("pids", pids))
	}
}

// TestHandler tests the handler configurarion by sending test messages.
func (h *myHandler) TestHandler() { proContainer.logger.Info("Test") }

func (h *myHandler) Init(conf *kw_config.Config) error {
	client, err := getClient()
	if err != nil {
		proContainer.logger.Error(err.Error(), zap.Error(err))
		return err
	}

	pods, err := client.CoreV1().Pods("").List(metav1.ListOptions{})
	if err != nil {
		proContainer.logger.Error(err.Error(), zap.Error(err))
		return err
	}

	for _, pod := range pods.Items {
		name := fmt.Sprintf("%s/%s", pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
		pids := make([]int, 0)

		for _, container := range pod.Status.ContainerStatuses {
			if len(container.ContainerID) > 0 {
				pid, err := getPodPID(&pod, container.ContainerID[9:])
				if err == nil && pid != -1 {
					pids = append(pids, pid)
				}
			}
		}

		if len(pids) > 0 {
			h.setter.SetContainerPIDs(name, pids)
		} else {
			h.setter.SetContainerPIDs(name, nil)
		}
	}

	return nil
}

func StartContainerProvider(setter *info.Setter, logger *zap.Logger) error {
	conf := &kw_config.Config{
		Resource:  kw_config.Resource{Pod: true},
		Namespace: "",
	}

	// 初始化配置
	proContainer = ContainerProvider{
		handler: &myHandler{setter: setter},
		logger:  logger,
	}
	// 获取初始全局配置
	proContainer.handler.Init(conf)
	// 运行变更监听
	go kw_controller.Start(conf, proContainer.handler)

	// logger.Info("start", zap.Any("pid_map", proContainer.handler.pids))

	return nil
}

func getClient() (kubernetes.Interface, error) {
	// 获取k8s配置
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = os.Getenv("HOME") + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, err
		}
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		proContainer.logger.Info(err.Error(), zap.Error(err))
		return nil, err
	}
	return client, nil
}

func getPodPID(pod *api_v1.Pod, containerID string) (int, error) {
	CgroupParent := fmt.Sprintf("kubepods/%s/pod%v/%s",
		strings.ToLower(fmt.Sprintf("%v", pod.Status.QOSClass)),
		pod.ObjectMeta.UID, containerID,
	)
	pidsCgroupPath := fmt.Sprintf("/sys/fs/cgroup/pids/%s/cgroup.procs", CgroupParent)
	_, err := os.Stat(pidsCgroupPath)
	if err == nil || os.IsExist(err) {
		// proContainer.logger.Info("pid", zap.String("path", pidsCgroupPath))
		output, err := ioutil.ReadFile(pidsCgroupPath)
		if err != nil {
			return -1, err
		}

		strPids := strings.Split(strings.Trim(string(output), " \n"), "\n")
		for _, s := range strPids {
			// 获取自己的信息
			pid, err := strconv.Atoi(strings.Trim(s, " \n"))
			if err != nil {
				return -1, err
			}
			return pid, nil
		}
	}

	return -1, nil
}
