package handler

import (
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"strconv"
)

func init() {
	resourceHandlers = append(resourceHandlers,
		&ResourceHandler{
			Kind:              "pod",
			ListWatchFactory:  getPodListWatcher,
			ObjectTypeFactory: func() runtime.Object { return &apiv1.Pod{} },
			HeaderCols:        []string{"NAME", "READY", "STATUS", "RESTARTS", "AGE"},
			SummaryCols:       getPodColumns,
		})
}

func getPodListWatcher(client *kubernetes.Clientset, namespace string) cache.ListerWatcher {
	return cache.NewListWatchFromClient(
		client.CoreV1().RESTClient(),
		"pods",
		namespace,
		fields.Everything())
}

func getPodColumns(obj interface{}) []string {
	pod := obj.(*apiv1.Pod)
	var restartCount int32 = 0
	var readyCount = 0
	for _, containerStatus := range pod.Status.ContainerStatuses {
		restartCount += containerStatus.RestartCount
		if containerStatus.Ready {
			readyCount++
		}
	}
	return []string{
		pod.Name,
		strconv.Itoa(readyCount) + "/" + strconv.Itoa(len(pod.Spec.Containers)),
		string(pod.Status.Phase),
		strconv.Itoa(int(restartCount)),
		calculateAge(pod.CreationTimestamp.Time),
	}
}
