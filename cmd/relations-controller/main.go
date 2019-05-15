package main

import (
	"os"
	"os/signal"
	"syscall"

	log "github.com/Sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
)

// retrieve the Kubernetes cluster client from outside of the cluster
func getKubernetesClient() kubernetes.Interface {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Infof("getClusterConfig: %v", err)

		kubeConfigPath := os.Getenv("HOME") + "/.kube/config"
		// create the config from the path
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		if err != nil {
			log.Fatalf("getClusterConfig: %v", err)
		}
	}

	// generate the client based off of the config
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("getClusterConfig: %v", err)
	}

	log.Info("Successfully constructed k8s client")
	return client
}

// main code path
func main() {
	// get the Kubernetes client for connectivity
	client := getKubernetesClient()

	// create the informer so that we can not only list resources
	// but also watch them for all services in the default namespace
	serviceInformer := cache.NewSharedIndexInformer(
		// the ListWatch contains two different functions that our
		// informer requires: ListFunc to take care of listing and watching
		// the resources we want to handle
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list all of the services (core resource) in the k8s-tengu-test namespace
				return client.CoreV1().Services("k8s-tengu-test").List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				// watch all of the services (core resource) in the k8s-tengu-test namespace
				return client.CoreV1().Services("k8s-tengu-test").Watch(options)
			},
		},
		&apiv1.Service{}, // the target type (Service)
		0,                // no resync (period of 0)
		cache.Indexers{},
	)
	deploymentInformer := cache.NewSharedIndexInformer(
		// the ListWatch contains two different functions that our
		// informer requires: ListFunc to take care of listing and watching
		// the resources we want to handle
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				// list all of the deployment (core resource) in the k8s-tengu-test namespace
				return client.AppsV1().Deployments("k8s-tengu-test").List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				// watch all of the deployment (core resource) in the k8s-tengu-test namespace
				return client.AppsV1().Deployments("k8s-tengu-test").Watch(options)
			},
		},
		&appsv1.Deployment{}, // the target type (Pod)
		0,                    // no resync (period of 0)
		cache.Indexers{},
	)

	// create a new queue so that when the informer gets a resource that is either
	// a result of listing or watching, we can add an idenfitying key to the queue
	// so that it can be handled in the handler
	serviceQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	deploymentQueue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	// add event handlers to handle the three types of events for resources:
	//  - adding new resources
	//  - updating existing resources
	//  - deleting resources
	serviceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// convert the resource object into a key (in this case
			// we are just doing it in the format of 'namespace/name')
			key, err := cache.MetaNamespaceKeyFunc(obj)
			log.Infof("Add service: %s", key)
			if err == nil {
				// add the key to the queue for the handler to get
				serviceQueue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			log.Infof("Update service: %s", key)
			if err == nil {
				serviceQueue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// DeletionHandlingMetaNamsespaceKeyFunc is a helper function that allows
			// us to check the DeletedFinalStateUnknown existence in the event that
			// a resource was deleted but it is still contained in the index
			//
			// this then in turn calls MetaNamespaceKeyFunc
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			log.Infof("Delete service: %s", key)
			if err == nil {
				serviceQueue.Add(key)
			}
		},
	})

	deploymentInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			// convert the resource object into a key (in this case
			// we are just doing it in the format of 'namespace/name')
			key, err := cache.MetaNamespaceKeyFunc(obj)
			log.Infof("Add pod: %s", key)
			if err == nil {
				// add the key to the queue for the handler to get
				deploymentQueue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			log.Infof("Update pod: %s", key)
			if err == nil {
				deploymentQueue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			// DeletionHandlingMetaNamsespaceKeyFunc is a helper function that allows
			// us to check the DeletedFinalStateUnknown existence in the event that
			// a resource was deleted but it is still contained in the index
			//
			// this then in turn calls MetaNamespaceKeyFunc
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			log.Infof("Delete pod: %s", key)
			if err == nil {
				deploymentQueue.Add(key)
			}
		},
	})

	// construct the Controller object which has all of the necessary components to
	// handle logging, connections, informing (listing and watching), the queue,
	// and the handler
	serviceController := Controller{
		logger:    log.NewEntry(log.New()),
		clientset: client,
		informer:  serviceInformer,
		queue:     serviceQueue,
		handler: &TestHandler{
			clientset: client,
		},
	}

	deploymentController := Controller{
		logger:    log.NewEntry(log.New()),
		clientset: client,
		informer:  deploymentInformer,
		queue:     deploymentQueue,
		handler: &TestHandler{
			clientset: client,
		},
	}

	// use a channel to synchronize the finalization for a graceful shutdown
	stopCh := make(chan struct{})
	defer close(stopCh)

	// run the controller loop to process items
	go serviceController.Run(stopCh)
	go deploymentController.Run(stopCh)

	// use a channel to handle OS signals to terminate and gracefully shut
	// down processing
	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm
}
