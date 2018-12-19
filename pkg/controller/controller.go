package controller

import (
	"fmt"
	"log"
	"time"

	clientset "github.com/tahsinrahman/hello-kube-crd-controller/pkg/client/clientset/versioned"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"

	crd_informer "github.com/tahsinrahman/hello-kube-crd-controller/pkg/client/informers/externalversions/samplecontroller.com/v1alpha1"
	crd_lister "github.com/tahsinrahman/hello-kube-crd-controller/pkg/client/listers/samplecontroller.com/v1alpha1"
	deploy_informer "k8s.io/client-go/informers/apps/v1"
	deploy_lister "k8s.io/client-go/listers/apps/v1"
)

// Controller defines the specifications of the controller
type Controller struct {
	kubeClientset kubernetes.Interface
	crdClientset  clientset.Interface

	deploymentLister deploy_lister.DeploymentLister
	fooLister        crd_lister.FooLister

	deploymentSyncer cache.InformerSynced
	fooSyncer        cache.InformerSynced

	queue workqueue.RateLimitingInterface
}

// NewController creates a new controller and returns the pointer to that controller
func NewController(
	kubeClientset kubernetes.Interface,
	crdClientset clientset.Interface,
	deploymentInformer deploy_informer.DeploymentInformer,
	fooInformer crd_informer.FooInformer,
) *Controller {

	log.Println("creating new controller")

	controller := &Controller{
		kubeClientset: kubeClientset,
		crdClientset:  crdClientset,

		deploymentLister: deploymentInformer.Lister(),
		fooLister:        fooInformer.Lister(),

		deploymentSyncer: deploymentInformer.Informer().HasSynced,
		fooSyncer:        fooInformer.Informer().HasSynced,

		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Foos"),
	}

	log.Println("setting up event handlers")

	fooInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueFoo,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueFoo(new)
		},
		DeleteFunc: controller.enqueueFoo,
	})
	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*appsv1.Deployment)
			oldDepl := old.(*appsv1.Deployment)

			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				return
			}

			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

func (c *Controller) handleObject(obj interface{}) {
	object, ok := obj.(metav1.Object)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			fmt.Println("error decoding object")
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			fmt.Println("error decoding tombstone object")
			return
		}
		fmt.Printf("recovered deleted object: %v\n", object.GetName())
	}
	fmt.Printf("processing object: %v\n", object.GetName())

	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		if ownerRef.Kind != "Foo" {
			return
		}

		foo, err := c.crdClientset.SamplecontrollerV1alpha1().Foos(object.GetNamespace()).Get(object.GetName(), metav1.GetOptions{})
		if err != nil {
			fmt.Printf("error while processing object: %v\n", object.GetName())
			return
		}
		c.enqueueFoo(foo)
	}
}

func (c *Controller) enqueueFoo(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		fmt.Printf("can't enqueu object %v\n", key)
		return
	}
	c.queue.AddRateLimited(key)
}

// Run waits for caches to be synced and then starts the worker goroutines
func (c *Controller) Run(threadiness int, stopCh chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	fmt.Println("starting foo controller")

	fmt.Println("waiting for caches to be synced")

	if ok := cache.WaitForCacheSync(stopCh, c.deploymentSyncer, c.fooSyncer); !ok {
		return fmt.Errorf("failed to wait for caches to synced")
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	fmt.Println("started workers")

	<-stopCh

	fmt.Println("workers stopped")

	return nil
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func (c *Controller) processNextItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}

	err := func(key string) error {
		defer c.queue.Done(key)

		if err := c.syncItem(key); err != nil {
			c.queue.AddRateLimited(key)
			return fmt.Errorf("error syncing item %v", key)
		}

		c.queue.Forget(key)
		fmt.Println("successfully synced")

		return nil
	}(key.(string))

	if err != nil {
		utilruntime.HandleError(err)
	}

	return true
}

func (c *Controller) syncItem(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return err
	}

	// get the foo resource with this name and namespace
	foo, err := c.fooLister.Foos(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Printf("object %v no longer exists in queue", name)
			return nil
		}
		return err
	}

	return nil
}
