package main

import (
	"context"
	"fmt"
	"informerTemplate/pkg/process"
	"informerTemplate/utils"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/klog/v2"
	"net/http"
	"time"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

var (
	kubeClient      *kubernetes.Clientset
	PodInformer     cache.SharedIndexInformer
	podListerSynced cache.InformerSynced
	dynamicClient   *dynamic.DynamicClient
)

func main() {
	// Initialize Kubernetes client
	var err error
	kubeClient, dynamicClient, err = utils.InitializeKubeClient()
	if err != nil {
		panic(err.Error())
	}

	// Start leader election
	utils.StartLeaderElection(kubeClient, runController)
}

// runController contains the main logic of the controller
func runController(ctx context.Context) {
	// 创建 SharedInformerFactory
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, time.Second*30)

	// 创建 Pod informer
	PodInformer = kubeInformerFactory.Core().V1().Pods().Informer()
	podListerSynced = PodInformer.HasSynced

	// 定义你的 CRD GroupVersionResource
	gvr := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "examples"}
	// 创建 Informer 来监听特定的 CRD 资源
	klog.Info("Creating informer for MyResource")
	klog.Info("GVR: ", gvr)
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(dynamicClient, 0, "", nil)
	myResourceInformer := factory.ForResource(gvr).Informer()

	//// 这样写只能对于内建的资源
	//// 为 MyResource 创建 informer是的不对的
	//myTempResourceInformer, _ := kubeInformerFactory.ForResource(
	//	schema.GroupVersionResource(metav1.GroupVersionResource{
	//		Group:    "mygroup.example.com",
	//		Version:  "v1",
	//		Resource: "myresources",
	//	}))
	//myTempResourceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{})

	// 创建工作队列
	addQueue := workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(),
		workqueue.RateLimitingQueueConfig{
			Name: "MyResourceAdd",
		})
	updateQueue := workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(),
		workqueue.RateLimitingQueueConfig{
			Name: "MyResourceUpdate",
		})
	deleteQueue := workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(),
		workqueue.RateLimitingQueueConfig{
			Name: "MyResourceDelete",
		})

	// 添加事件处理器
	myResourceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				addQueue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				updateQueue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				deleteQueue.Add(key)
			}
		},
	})

	// 启动 informer 并等待同步
	stopCh := make(chan struct{})
	defer close(stopCh)

	kubeInformerFactory.Start(stopCh)

	utils.Ready = true // 设置 Readiness 探针为 true

	config := process.Cfg{
		PodInformer: PodInformer,
	}

	// 启动协程处理 Add 事件
	go process.ProcessQueue(addQueue, "Add", config)

	// 启动协程处理 Update 事件
	go process.ProcessQueue(updateQueue, "Update", config)

	// 启动协程处理 Delete 事件
	go process.ProcessQueue(deleteQueue, "Delete", config)

	// 启动 HTTP 服务器，用于 Readiness 和 Liveness 探针
	http.HandleFunc("/readiness", utils.ReadinessHandler)
	http.HandleFunc("/liveness", utils.LivenessHandler)
	go http.ListenAndServe(":8080", nil)

	if !cache.WaitForCacheSync(stopCh, podListerSynced, myResourceInformer.HasSynced) {
		fmt.Println("Timed out waiting for caches to sync")
		return
	}

	// 等待停止信号
	<-ctx.Done()
}
