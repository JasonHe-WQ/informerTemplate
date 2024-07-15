package utils

import (
	"fmt"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"os"
)

// InitializeKubeClient initializes a Kubernetes clientset
func InitializeKubeClient() (*kubernetes.Clientset, *dynamic.DynamicClient, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			home := os.Getenv("HOME")
			kubeconfig = fmt.Sprintf("%s/.kube/config", home)
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			log.Fatalf("Error loading kubeconfig: %v", err)
		}
	}

	KubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	DynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}

	return KubeClient, DynamicClient, nil
}
