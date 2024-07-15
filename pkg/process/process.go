package process

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
)

type Cfg struct {
	PodInformer cache.SharedIndexInformer
}

const maxRetries = 5

// 处理队列中的 item
func ProcessQueue(queue workqueue.RateLimitingInterface, eventType string, config Cfg) {
	for {
		key, shutdown := queue.Get()
		if shutdown {
			break
		}

		// 处理 key
		if err := processItem(key.(string), eventType, config); err != nil {
			// 如果处理失败，重新入队
			if queue.NumRequeues(key) < maxRetries {
				klog.Errorf("Error processing %s event for key %v: %v", eventType, key, err)
				queue.AddRateLimited(key)
			} else {
				// 超过最大重试次数，放弃
				klog.Errorf("Dropping %s event for key %v out of the queue: %v", eventType, key, err)
				queue.Forget(key)
			}
		} else {
			// 处理成功，从队列中移除
			queue.Forget(key)
		}

		queue.Done(key)
	}
}

// 处理 item 的实际逻辑
func processItem(key, eventType string, config Cfg) error {
	// 列出带有指定标签的 Pod
	labelSelector := "app=myapp" // 替换为你需要的标签选择器
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{"app": "myapp"},
	})
	if err != nil {
		return fmt.Errorf("error creating label selector: %v", err)
	}

	PodInformer := config.PodInformer
	pods, err := PodInformer.GetIndexer().ByIndex(cache.NamespaceIndex, "")
	if err != nil {
		return fmt.Errorf("error listing pods: %v", err)
	}

	fmt.Printf("Processing %s event for key: %v\n", eventType, key)
	fmt.Printf("Found %d pods with label %s:\n", len(pods), labelSelector)
	for _, obj := range pods {
		pod := obj.(*v1.Pod)
		if selector.Matches(labels.Set(pod.Labels)) {
			fmt.Printf("- %s/%s\n", pod.Namespace, pod.Name)
		}
	}

	return nil
}
