package handler

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/duration"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"time"
)

var resourceHandlers = make([]*ResourceHandler,0)

var resourceTypes []string
var resourceHandlerMap map[string]*ResourceHandler

type ResourceHandler struct {
    Kind              string
    ListWatchFactory  func(client *kubernetes.Clientset, namespace string) cache.ListerWatcher
    ObjectTypeFactory func() runtime.Object
    SummaryCols       func(obj interface{}) []string
    HeaderCols        []string
}

func GetResourceTypes() []string {
	if resourceTypes == nil {
		resourceTypes = make([]string,0,len(resourceHandlers))
		for _, handler := range resourceHandlers {
			resourceTypes = append(resourceTypes, handler.Kind)
		}
	}
	return resourceTypes
}

func GetResourceHandler(kind string) *ResourceHandler {
	if resourceHandlerMap == nil {
		resourceHandlerMap = make(map[string]*ResourceHandler)
		for _, handler := range resourceHandlers {
			resourceHandlerMap[handler.Kind] = handler
		}
	}
	return resourceHandlerMap[kind]
}

// ===============================================================================================

func calculateAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return duration.ShortHumanDuration(time.Now().Sub(t))
}

