package utils

import (
	"context"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog/v2"
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
)

// StartLeaderElection starts the leader election process
func StartLeaderElection(kubeClient *kubernetes.Clientset, runFunc func(context.Context)) {
	id, err := os.Hostname()
	if err != nil {
		klog.Fatalf("Error getting hostname: %v", err)
	}

	lock := &resourcelock.LeaseLock{
		LeaseMeta: metav1.ObjectMeta{
			Name:      "my-controller-leader-election",
			Namespace: "default", // Replace with the namespace you want to use
		},
		Client: kubeClient.CoordinationV1(),
		LockConfig: resourcelock.ResourceLockConfig{
			Identity: id,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	leaderElectionConfig := leaderelection.LeaderElectionConfig{
		LeaseDuration: time.Second * 15,
		RenewDeadline: time.Second * 10,
		RetryPeriod:   time.Second * 2,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(c context.Context) {
				klog.Info("Became the leader, starting control loop")
				runFunc(c)
			},
			OnStoppedLeading: func() {
				klog.Info("Stopped leading")
				cancel()
			},
		},
		Lock: lock,
	}

	leaderElector, err := leaderelection.NewLeaderElector(leaderElectionConfig)
	if err != nil {
		klog.Fatalf("Error creating leader elector: %v", err)
	}

	leaderElector.Run(ctx)
}
