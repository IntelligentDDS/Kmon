package provider

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bitnami-labs/kubewatch/pkg/handlers"
	"github.com/bitnami-labs/kubewatch/pkg/utils"
	"github.com/sirupsen/logrus"
	api_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"

	"go.uber.org/zap"
	"k8s.io/client-go/rest"

	"gitlab.dds-sysu.tech/Wuny/kmon/src/model"
)

var logger, _ = zap.NewDevelopment().Sugar()
var actionsPIDChange map[model.Collector][]func(model.Collector, *EventPIDChange) = map[model.Collector][]func(model.Collector, *EventPIDChange){}
var deferPIDChange map[*EventPIDChange][]func(model.Collector, *EventPIDChange) = map[*EventPIDChange][]func(model.Collector, *EventPIDChange){}

type ServicePID struct {
	src       *model.ProviderConfig
	service   map[string][]string // key: nameaspace, value: service
	pids      map[uint32]podInfo  // key: pid, value: PodInfo
	update    bool
	clientset *kubernetes.Clientset
}

type podInfo struct {
	serviceName   string
	podName       string
	containerName string
}

func init() {
	// providers["service"] = NewServicePID
}

type EventPIDChange struct {
	AddPID []uint32
	DelPID []uint32
	AllPID []uint32
}

type dataStore struct {
	pids        map[int]podInfo // key: pid, value: PodInfo
	listener    map[servicePair][]listenPair
	serviceList map[string]map[string]bool
	clientset   *kubernetes.Clientset
}

type servicePair struct {
	namespace string
	service   string
}

type listenPair struct {
	action     func(model.Collector, *EventPIDChange)
	controller model.Collector
}

var data dataStore = dataStore{
	pids:        make(map[int]podInfo),
	listener:    make(map[servicePair][]listenPair),
	serviceList: make(map[string]map[string]bool),
}

func ListenPIDChange(serviceMap map[string][]string, controller model.Collector, action func(model.Collector, *EventPIDChange)) {
	for namespace, services := range serviceMap {
		spaceList, ok := data.serviceList[namespace]
		if !ok {
			spaceList = make(map[string]bool)
			data.serviceList[namespace] = spaceList
		}

		for _, serviceName := range services {
			pair := servicePair{
				namespace: namespace,
				service:   serviceName,
			}

			pairList, ok := data.listener[pair]
			if !ok {
				pairList = make([]listenPair, 0)
				data.listener[pair] = pairList
			}

			pairList = append(pairList, listenPair{controller: controller, action: action})
			spaceList[serviceName] = true
		}
	}
}

func RemovePIDChange(serviceMap map[string][]string, controller model.Collector) {
	for namespace, services := range serviceMap {
		spaceList, ok := data.serviceList[namespace]
		if !ok {
			continue
		}

		for _, serviceName := range services {
			pair := servicePair{
				namespace: namespace,
				service:   serviceName,
			}
			pairList, ok := data.listener[pair]
			if !ok {
				break
			}

			for index := range pairList {
				if pairList[index].controller == controller {
					pairList = append(pairList[:index], pairList[index+1:]...)
					break
				}
			}
			delete(spaceList, serviceName)
		}
	}
}

func EmitPIDChange(sender model.Collector, event *EventPIDChange) {
	deferPIDChange[event] = make([]func(model.Collector, *EventPIDChange), 0)

	if actions, ok := actionsPIDChange[sender]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	if actions, ok := actionsPIDChange[nil]; ok {
		for _, action := range actions {
			action(sender, event)
		}
	}

	deferAction := deferPIDChange[event]
	delete(deferPIDChange, event)
	for i := len(deferAction) - 1; i >= 0; i-- {
		deferAction[i](sender, event)
	}
}

func DeferPIDChange(event *EventPIDChange, action func(model.Collector, *EventPIDChange)) {
	if actions, ok := deferPIDChange[event]; ok {
		deferPIDChange[event] = append(actions, action)
	} else {
		logger.Error("Error to register defer function to unknown event")
	}
}

func NewServicePID(src *model.ProviderConfig) (model.Provider, error) {
	var kubeClient kubernetes.Interface
	client, err := getClient()
	if err != nil {
		return nil, fmt.Errorf("Init k8s client failed: %w", err)
	}
	data.clientset = client

	nodeNotReadyInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = "involvedObject.kind=Node,type=Normal,reason=NodeNotReady"
				return kubeClient.CoreV1().Events().List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = "involvedObject.kind=Node,type=Normal,reason=NodeNotReady"
				return kubeClient.CoreV1().Events().Watch(options)
			},
		},
		&api_v1.Event{},
		0, //Skip resync
		cache.Indexers{},
	)
	nodeNotReadyController := newResourceController(kubeClient, eventHandler, nodeNotReadyInformer, "NodeNotReady")
	stopNodeNotReadyCh := make(chan struct{})
	defer close(stopNodeNotReadyCh)
	go nodeNotReadyController.Run(stopNodeNotReadyCh)

	nodeReadyInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = "involvedObject.kind=Node,type=Normal,reason=NodeReady"
				return kubeClient.CoreV1().Events().List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = "involvedObject.kind=Node,type=Normal,reason=NodeReady"
				return kubeClient.CoreV1().Events().Watch(options)
			},
		},
		&api_v1.Event{},
		0, //Skip resync
		cache.Indexers{},
	)

	nodeReadyController := newResourceController(kubeClient, eventHandler, nodeReadyInformer, "NodeReady")
	stopNodeReadyCh := make(chan struct{})
	defer close(stopNodeReadyCh)

	go nodeReadyController.Run(stopNodeReadyCh)

	informer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				return kubeClient.CoreV1().Pods(conf.Namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				return kubeClient.CoreV1().Pods(conf.Namespace).Watch(options)
			},
		},
		&api_v1.Pod{},
		0, //Skip resync
		cache.Indexers{},
	)

	c := newResourceController(kubeClient, eventHandler, informer, "pod")
	stopCh := make(chan struct{})
	defer close(stopCh)

	go c.Run(stopCh)

	// For Capturing CrashLoopBackOff Events in pods
	backoffInformer := cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options meta_v1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = "involvedObject.kind=Pod,type=Warning,reason=BackOff"
				return kubeClient.CoreV1().Events(conf.Namespace).List(options)
			},
			WatchFunc: func(options meta_v1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = "involvedObject.kind=Pod,type=Warning,reason=BackOff"
				return kubeClient.CoreV1().Events(conf.Namespace).Watch(options)
			},
		},
		&api_v1.Event{},
		0, //Skip resync
		cache.Indexers{},
	)

	backoffcontroller := newResourceController(kubeClient, eventHandler, backoffInformer, "Backoff")
	stopBackoffCh := make(chan struct{})
	defer close(stopBackoffCh)

	go backoffcontroller.Run(stopBackoffCh)

	go func() {
		coreV1 := data.clientset.CoreV1()
		for {
			for namespace, serviceList := range data.serviceList {
				services, err := coreV1.Services(namespace).List(meta_v1.ListOptions{})
				if err != nil {
					logger.Error("Get info of services failed!", zap.String("Namespace", namespace))
				}

				for _, serviceItem := range services.Items {
					if _, ok := serviceList[serviceItem.Name]; !ok {
						continue
					}

					pods, err := coreV1.Pods(namespace).List(meta_v1.ListOptions{
						LabelSelector: labels.Set(serviceItem.Spec.Selector).String(),
					})
					if err != nil {
						logger.Info("Cannot find pod", zap.Error(err))
						continue
					}

					for _, pod := range pods.Items {
						for _, container := range pod.Status.ContainerStatuses {
							pid, err := getPodPID(pod, container.ContainerID[9:])
							if pid == -1 && err != nil {
								continue
							}

							info, ok := data.pids[pid]
							if ok && (info.serviceName != serviceItem.Name ||
								info.podName != pod.ObjectMeta.Name ||
								info.containerName != container.Name) {

								info.serviceName = serviceItem.Name
								info.podName = pod.ObjectMeta.Name
								info.containerName = container.Name
							}
							data.pids[pid] = info
						}
					}
				}
			}
			time.Sleep(5 * time.Second)
		}
	}()
	return nil, nil
}

func AppendAdditionalData(expData *model.ExportData) {
	pidInterface, ok := expData.Additional["pid"]
	if !ok {
		return
	}
	var pid int
	if pidVal, ok := pidInterface.(int); ok {
		pid = pidVal
	} else if pidVal, ok := pidInterface.(uint32); ok {
		pid = int(pidVal)
	} else if pidVal, ok := pidInterface.(uint); ok {
		pid = int(pidVal)
	} else if pidVal, ok := pidInterface.(int32); ok {
		pid = int(pidVal)
	} else {
		return
	}

	if info, ok := data.pids[pid]; ok {
		expData.Tags["service"] = info.serviceName
		expData.Tags["pod"] = info.podName
		expData.Tags["container"] = info.containerName
	}
}

func buildOutOfClusterConfig() (*rest.Config, error) {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("HOME") + "/.kube/config"
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

func getClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		config, err = buildOutOfClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("Init the ClusterSpec using k8s config failed: %w", err)
		}
	}

	return kubernetes.NewForConfig(config)
}

func newResourceController(client kubernetes.Interface, eventHandler handlers.Handler, informer cache.SharedIndexInformer, resourceType string) *Controller {
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	var newEvent Event
	var err error
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(obj)
			newEvent.eventType = "create"
			newEvent.resourceType = resourceType
			logrus.WithField("pkg", "kubewatch-"+resourceType).Infof("Processing add to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
		UpdateFunc: func(old, new interface{}) {
			newEvent.key, err = cache.MetaNamespaceKeyFunc(old)
			newEvent.eventType = "update"
			newEvent.resourceType = resourceType
			logrus.WithField("pkg", "kubewatch-"+resourceType).Infof("Processing update to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
		DeleteFunc: func(obj interface{}) {
			newEvent.key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			newEvent.eventType = "delete"
			newEvent.resourceType = resourceType
			newEvent.namespace = utils.GetObjectMetaData(obj).Namespace
			logrus.WithField("pkg", "kubewatch-"+resourceType).Infof("Processing delete to %v: %s", resourceType, newEvent.key)
			if err == nil {
				queue.Add(newEvent)
			}
		},
	})

	return &Controller{
		logger:       logrus.WithField("pkg", "kubewatch-"+resourceType),
		clientset:    client,
		informer:     informer,
		queue:        queue,
		eventHandler: eventHandler,
	}
}

func getPodPID(pod kubernetes.Pod, containerID string) (int, error) {
	CgroupParent := fmt.Sprintf("kubepods/%s/pod%v/%s",
		strings.ToLower(fmt.Sprintf("%v", pod.Status.QOSClass)),
		pod.ObjectMeta.UID, containerID,
	)
	pidsCgroupPath := fmt.Sprintf("/sys/fs/cgroup/pids/%s/cgroup.procs", CgroupParent)
	// ppidMap := make(map[string]bool)
	_, err := os.Stat(pidsCgroupPath)
	if err == nil || os.IsExist(err) {
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

func getPPID(pid int) (int, error) {
	ppidExp, err := regexp.Compile("PPid:\t(\\d+)")
	pidStatus := fmt.Sprintf("/proc/%d/status", pid)

	status, err := ioutil.ReadFile(pidStatus)
	ppidMatch := ppidExp.FindSubmatch(status)
	if err != nil {
		return -1, err
	}

	ppidStr := string(ppidMatch[1])
	ppid, err := strconv.Atoi(ppidStr)
	if err != nil {
		return -1, err
	}
	return ppid, nil
}

// // https://github.com/bitnami-labs/kubewatch/blob/84a34db93ff9935ce133f4eb1175187154253685/pkg/controller/controller.go#L511
