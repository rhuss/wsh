package main

import (
    "fmt"
    "github.com/abiosoft/ishell"
    "github.com/fatih/color"
    "github.com/rhuss/wsh/pkg/handler"
    "github.com/sirupsen/logrus"
    "gopkg.in/yaml.v2"
    "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
    restclient "k8s.io/client-go/rest"
    "k8s.io/client-go/tools/cache"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/util/workqueue"
    "os"
    "strings"
    "text/tabwriter"
    "time"
)

type ShellContext struct {
    CurrentNamespace string
    Kind string
    Level int
}

var server = "minikube"

var shell *ishell.Shell

// Getter interface knows how to access Get method from RESTClient.
type Getter interface {
	Get() *restclient.Request
}

var informerCache = map[string]map[string]cache.SharedIndexInformer{}

var shellContext = ShellContext{}
var client *kubernetes.Clientset

// Init watcher to watch pods
var queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

var stopCh = make(chan struct{})

var resourceCache = make(map[string]map[string]map[string]interface{})

func main(){
    client = GetClientOutOfCluster()
    defer close(stopCh)

    // Start a shell
    shell = ishell.New()
    updateContext()

    shell.AddCmd(&ishell.Cmd{
        Name: "cd",
        Help: "Change context",
        Func: func(c *ishell.Context) {
            if c.Args[0] == ".." {
                shellContext.Level -= 1
                if shellContext.Level <= 1 {
                    shellContext.Kind = ""
                }

                if shellContext.Level <= 0 {
                    shellContext.Level = 0
                    shellContext.CurrentNamespace = ""
                }
            } else if shellContext.Level == 0 {
                if err := verifyNamespace(c.Args[0]); err == nil {
                    shellContext.CurrentNamespace = c.Args[0]
                    shellContext.Level = 1
                } else {
                    println("No namespace " + c.Args[0])
                }
            } else if shellContext.Level == 1 {
                if err := verifyResourceKind(c.Args[0]); err == nil {
                    shellContext.Kind = c.Args[0]
                    shellContext.Level = 2
                } else {
                    println("Unknown resource type " + c.Args[0])
                }
            }
            updateContext()
        },

        Completer: func([]string) []string {
            if shellContext.Level == 0 {
                if ret, err := listNamespaces(); err == nil {
                    return ret
                }
            } else if shellContext.Level == 1 {
                return append(handler.GetResourceTypes(), "..")
            } else if shellContext.Level == 2 {
                return []string { ".." }
            }
            return nil
        },
    })

    shell.AddCmd(&ishell.Cmd{
        Name: "ls",
        Help: "Show resources",
        Func: func(c *ishell.Context) {
            if shellContext.Level == 0 {
                printNamespaces()
            } else if shellContext.Level == 1 {
                printResourceKinds()
            } else if shellContext.Level == 2 {
                printResources(shellContext.CurrentNamespace, shellContext.Kind, c.Args)
            }
        },
    })

    shell.AddCmd(&ishell.Cmd{
        Name: "cat",
        Help: "Show resource context",
        Func: func(c *ishell.Context) {
            if shellContext.Level != 2 {
                println("Can only run from within a resource context")
                return
            }

            if len(c.Args) < 1 {
                println("You have to provide a resource name to print out")
                return
            }

            moreThanOne := len(c.Args) > 1
            for _, resource := range c.Args {
                if moreThanOne {
                    println(">>>>> " + resource)
                }
                catResource(shellContext.CurrentNamespace, shellContext.Kind, resource)
            }
        },
        Completer: func(args []string) []string {
            if shellContext.Level == 2 {
                return listResourceKeys(shellContext.CurrentNamespace, shellContext.Kind, args)
            }
            return nil
        },
    })
    shell.Run()
}

func ensureInformer(namespace string, handler *handler.ResourceHandler) {
    if informerCache[namespace] == nil {
        informerCache[namespace] = make(map[string]cache.SharedIndexInformer)
    }

    if informerCache[namespace][handler.Kind] != nil {
        return
    }

    informer := cache.NewSharedIndexInformer(
        handler.ListWatchFactory(client, namespace),
        handler.ObjectTypeFactory(),
        10 * time.Second, //Skip resync
        cache.Indexers{},
    )

    cacheAddFunc := func(obj interface{}) {
        name, err := getName(obj)
        if err != nil {
            return
        }
        nsMap := resourceCache[namespace]
        if nsMap == nil {
            nsMap = make(map[string]map[string]interface{})
            resourceCache[namespace] = nsMap
        }
        kindMap := resourceCache[namespace][handler.Kind]
        if kindMap == nil {
            kindMap = make(map[string]interface{})
            resourceCache[namespace][handler.Kind] = kindMap
        }
        kindMap[name] = obj
    }

    informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
        AddFunc: cacheAddFunc,
        UpdateFunc: func(oldObj, newObj interface{}) {
            cacheAddFunc(newObj)
        },
        DeleteFunc: func(obj interface{}) {
            name, err := getName(obj)
            if err != nil {
                return
            }
            delete(resourceCache[namespace][handler.Kind],name)
        },
    })

    go informer.Run(stopCh)
}

func getName(obj interface{}) (string, error) {
    metaInfo, err := meta.Accessor(obj)
    if err != nil {
        return "", fmt.Errorf("object has no meta: %v", err)
    }
    return metaInfo.GetName(),nil
}


func verifyResourceKind(kind string) error {
    for _, resource := range handler.GetResourceTypes() {
        if kind == resource {
            return nil
        }
    }
    return fmt.Errorf("unknown resource kind %s", kind)
}

func printResourceKinds() {
    for _, kind := range handler.GetResourceTypes() {
        println(kind)
    }
}

func catResourceRaw(namespace string, kind string, podName string) {
    resource, err :=
        client.CoreV1().RESTClient().Get().Namespace(namespace).Resource(kind + "s").DoRaw()
    if err != nil {
        fmt.Printf("%v\n", err)
        return
    }
    println(string(resource[:]))
}

func catResource(namespace string, kind string, name string) {
    obj, err := client.CoreV1().RESTClient().Get().
        Namespace(namespace).
        Resource(kind + "s").
        Name(name).
        Do().
        Get()
    if err != nil {
        fmt.Printf("%v\n", err)
        return
    }
    out, err := yaml.Marshal(obj)
    if err != nil {
        fmt.Printf("%v\n", err)
        return
    }
    println(string(out[:]))
}

func verifyNamespace(ns string) error {
    _, err := client.CoreV1().Namespaces().Get(ns,metav1.GetOptions{})
    if err != nil {
        return err
    }
    return nil
}

func updateContext() {
    namespace := shellContext.CurrentNamespace
    ret := color.New(color.BgCyan, color.FgBlack).Sprint(server + " ")
    if namespace != "" {
        ret += color.New(color.BgYellow, color.FgCyan).Sprint("")
        ret += color.New(color.BgYellow, color.FgBlack).Sprint(" " + namespace + " ")
    }

    if shellContext.Kind != "" {
        ret += color.New(color.BgGreen, color.FgYellow).Sprint("")
        ret += color.New(color.BgGreen, color.FgBlack).Sprint(" " + shellContext.Kind + " ")
    }

    if shellContext.Level == 0 {
        ret += color.CyanString("") + " "
    } else if shellContext.Level == 1 {
        ret += color.YellowString("") + " "
    } else if shellContext.Level == 2 {
        ret += color.GreenString("") + " "
    }
    shell.SetPrompt(ret)

    if shellContext.Kind != "" {
        h := handler.GetResourceHandler(shellContext.Kind)
        if h == nil {
            return
        }
        ensureInformer(namespace, h)
    }
}

func printResources(namespace string, kind string, args []string) {

    resources := listResources(namespace, kind, args)
    h := handler.GetResourceHandler(kind)
    w := tabwriter.NewWriter(os.Stdout, 3, 4,3, ' ', tabwriter.TabIndent)
    defer w.Flush()

    fmt.Fprintln(w, strings.Join(h.HeaderCols,"\t"))
    for _, resource := range resources {
        fmt.Fprintln(w, strings.Join(h.SummaryCols(resource), "\t"))
    }
}

func listResources(namespace string, kind string, args []string) []interface{} {
    kindMap := getKindMap(namespace, kind)
    if kindMap == nil {
        return nil
    }
    resources := make([]interface{}, 0, len(kindMap))
    for _, resource := range kindMap {
        resources = append(resources, resource)
    }
    return resources
}

func listResourceKeys(namespace string, kind string, args []string) []string {
kindMap := getKindMap(namespace, kind)
    if kindMap == nil {
        return nil
    }
    resources := make([]string, 0, len(kindMap))
    for kindKey := range kindMap {
        resources = append(resources, kindKey)
    }
    return resources
}

func getKindMap(namespace string, kind string) map[string]interface{} {
    nsMap := resourceCache[namespace]
    if nsMap == nil {
        return nil
    }
    kindMap := resourceCache[namespace][kind]
    if kindMap == nil {
        return nil
    }
    return kindMap
}

func printNamespaces() {
    namespaces, err := listNamespaces()
    if err != nil {
        logrus.WithError(err).Error("Cannot list namespaces")
        return
    }
    for _, ns := range namespaces {
        println(ns)
    }
}

func listNamespaces() ([]string, error) {
    list, err := client.CoreV1().Namespaces().List(metav1.ListOptions{})
    if err != nil {
        return nil, err
    }
    ret := make([]string, 0, len(list.Items))
    for _, ns := range list.Items {
        ret = append(ret, ns.ObjectMeta.Name)
    }
    return ret,nil
}

func buildOutOfClusterConfig() (*rest.Config, error) {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("HOME") + "/.kube/config"
	}
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

// GetClientOutOfCluster returns a k8s clientset to the request from outside of cluster
func GetClientOutOfCluster() *kubernetes.Clientset {
    config, err := buildOutOfClusterConfig()
    if err != nil {
        logrus.Fatalf("Can not get kubernetes config: %v", err)
    }

    clientset, err := kubernetes.NewForConfig(config)

    return clientset
}