package handler

import (
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"strconv"
	"strings"
)

func init() {

	resourceHandlers = append(resourceHandlers,
		&ResourceHandler{
			Kind:              "service",
			ListWatchFactory:  getServiceListWatcher,
			ObjectTypeFactory: func() runtime.Object { return &apiv1.Service{} },
			HeaderCols:        []string{"NAME", "TYPE", "CLUSTER-IP", "EXTERNAL-IP", "PORT(S)", "AGE"},
			SummaryCols:       getServiceCols,
	})
}

func getServiceListWatcher(client *kubernetes.Clientset, namespace string) cache.ListerWatcher {
	return cache.NewListWatchFromClient(
		client.CoreV1().RESTClient(),
		"services",
		namespace,
		fields.Everything())
}

func getServiceCols(obj interface{}) []string {
	service := obj.(*apiv1.Service)
	var externalIps = "<none>"
	if len(service.Spec.ExternalIPs) > 0 {
		externalIps = strings.Join(service.Spec.ExternalIPs, ",")
	}

	var ports = "<none>"
	if len(service.Spec.Ports) > 0 {
		ports = ""
		for i, port := range service.Spec.Ports {
			ports += strconv.Itoa(int(port.Port)) + "/" + string(port.Protocol)
			if i != len(service.Spec.Ports)-1 {
				ports += ","
			}
		}
	}
	return []string{
		service.Name,
		string(service.Spec.Type),
		service.Spec.ClusterIP,
		externalIps,
		ports,
		calculateAge(service.CreationTimestamp.Time),
	}
}
