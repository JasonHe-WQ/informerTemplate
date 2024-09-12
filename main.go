package main

import (
	"context"
	"fmt"
	"informerTemplate/pkg/process"
	"informerTemplate/utils"
	"net/http"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/klog/v2"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	kubeClient      *kubernetes.Clientset
	PodInformer     cache.SharedIndexInformer
	podListerSynced cache.InformerSynced
	dynamicClient   *dynamic.DynamicClient

	// 定义 Prometheus 指标
	eventCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "myresource_events_total",
			Help: "Total number of MyResource events processed",
		},
		[]string{"event_type"},
	)

	queueDepthGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "myresource_queue_depth",
			Help: "Current depth of MyResource event queues",
		},
		[]string{"queue_name"},
	)

	processingDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "myresource_processing_duration_seconds",
			Help:    "Duration of MyResource event processing",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"event_type"},
	)
)

func init() {
	// 注册 Prometheus 指标
	prometheus.MustRegister(eventCounter)
	prometheus.MustRegister(queueDepthGauge)
	prometheus.MustRegister(processingDurationHistogram)
}

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
				eventCounter.WithLabelValues("add").Inc()
				klog.Infof("Add event: Added %s to AddQueue", key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				updateQueue.Add(key)
				eventCounter.WithLabelValues("update").Inc()
				klog.Infof("Update event: Added %s to UpdateQueue", key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err == nil {
				deleteQueue.Add(key)
				eventCounter.WithLabelValues("delete").Inc()
				klog.Infof("Delete event: Added %s to DeleteQueue", key)
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

	// 启动协程更新队列深度指标
	go func() {
		for {
			queueDepthGauge.WithLabelValues("add").Set(float64(addQueue.Len()))
			queueDepthGauge.WithLabelValues("update").Set(float64(updateQueue.Len()))
			queueDepthGauge.WithLabelValues("delete").Set(float64(deleteQueue.Len()))
			time.Sleep(5 * time.Second)
		}
	}()

	// 启动 HTTP 服务器，用于 Readiness 和 Liveness 探针以及 Prometheus 指标
	http.HandleFunc("/readiness", utils.ReadinessHandler)
	http.HandleFunc("/liveness", utils.LivenessHandler)
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":8080", nil)

	if !cache.WaitForCacheSync(stopCh, podListerSynced, myResourceInformer.HasSynced) {
		fmt.Println("Timed out waiting for caches to sync")
		return
	}

	// 等待停止信号
	<-ctx.Done()
	klog.Info("Controller stopped")
}
