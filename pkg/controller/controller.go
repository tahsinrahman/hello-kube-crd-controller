package controller

import (
	"log"

	clientset "github.com/tahsinrahman/hello-kube-crd-controller/pkg/client/clientset/versioned"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	appsv1 "k8s.io/api/apps/v1"
)

// Controller defines the specifications of the controller
type Controller struct {
	kubeClientset kubernetes.Interface
	crdClientset  clientset.Interface

	deploymentInformer cache.SharedIndexInformer
	crdInformer        cache.SharedIndexInformer

	queue workqueue.RateLimitingInterface
}

// NewController creates a new controller and returns the pointer to that controller
func NewController(
	kubeClientset kubernetes.Interface,
	crdClientset clientset.Interface,
	deploymentInformer cache.SharedIndexInformer,
	crdInformer cache.SharedIndexInformer,
) *Controller {

	log.Println("creating new controller")

	controller := &Controller{
		kubeClientset: kubeClientset,
		crdClientset:  crdClientset,

		deploymentInformer: deploymentInformer,
		crdInformer:        crdInformer,

		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Foos"),
	}

	log.Println("setting up event handlers")

	controller.crdInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
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
	/*
		object, ok := obj.(metav1.ObjectMeta)
		if !ok {
		}
	*/
}
