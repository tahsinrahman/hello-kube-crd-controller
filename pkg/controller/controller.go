package controller

import (
	"fmt"
	"log"
	"time"

	clientset "github.com/tahsinrahman/hello-kube-crd-controller/pkg/client/clientset/versioned"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"

	crd_informer "github.com/tahsinrahman/hello-kube-crd-controller/pkg/client/informers/externalversions/samplecontroller.com/v1alpha1"
	crd_lister "github.com/tahsinrahman/hello-kube-crd-controller/pkg/client/listers/samplecontroller.com/v1alpha1"
	deploy_informer "k8s.io/client-go/informers/apps/v1"
	deploy_lister "k8s.io/client-go/listers/apps/v1"

	sampleV1beta1 "github.com/tahsinrahman/hello-kube-crd-controller/pkg/apis/samplecontroller.com/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kutil_appsv1 "github.com/appscode/kutil/apps/v1"
)

// Controller defines the specifications of the controller
type Controller struct {
	kubeClientset kubernetes.Interface
	fooClientset  clientset.Interface

	deploymentLister deploy_lister.DeploymentLister
	deploymentSyncer cache.InformerSynced

	fooLister crd_lister.FooLister
	fooSyncer cache.InformerSynced
	queue     workqueue.RateLimitingInterface
}

// NewController creates a new controller and returns the pointer to that controller
func NewController(
	kubeClientset kubernetes.Interface,
	fooClientset clientset.Interface,
	deploymentInformer deploy_informer.DeploymentInformer,
	fooInformer crd_informer.FooInformer,
) *Controller {

	log.Println("creating new controller")

	controller := &Controller{
		kubeClientset: kubeClientset,
		fooClientset:  fooClientset,

		deploymentLister: deploymentInformer.Lister(),
		deploymentSyncer: deploymentInformer.Informer().HasSynced,

		fooLister: fooInformer.Lister(),
		fooSyncer: fooInformer.Informer().HasSynced,

		queue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
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
			log.Println("error decoding object")
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			log.Println("error decoding tombstone object")
			return
		}
		log.Printf("recovered deleted object: %v\n", object.GetName())
	}
	log.Printf("handling deployment: %v\n", object.GetName())

	log.Println("ownerRef: ", metav1.GetControllerOf(object))
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		log.Println(ownerRef)
		if ownerRef.Kind != "Foo" {
			return
		}

		foo, err := c.fooLister.Foos(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			log.Printf("error while processing object: %v\n", object.GetName())
			return
		}
		c.enqueueFoo(foo)
	}
}

func (c *Controller) enqueueFoo(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		log.Printf("can't enqueu object %v\n", key)
		return
	}
	log.Println("queueing object ", key)
	c.queue.AddRateLimited(key)
}

// Run waits for caches to be synced and then starts the worker goroutines
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	log.Println("starting foo controller")

	log.Println("waiting for caches to be synced")

	if ok := cache.WaitForCacheSync(stopCh, c.deploymentSyncer, c.fooSyncer); !ok {
		return fmt.Errorf("failed to wait for caches to synced")
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	log.Println("started workers")

	<-stopCh

	log.Println("workers stopped")

	return nil
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func (c *Controller) processNextItem() bool {
	log.Println("inside processedNextItem")
	obj, quit := c.queue.Get()
	if quit {
		return false
	}

	err := func(obj interface{}) error {
		defer c.queue.Done(obj)

		key, ok := obj.(string)
		if !ok {
			c.queue.Forget(key)
			return nil
		}

		if err := c.syncItem(key); err != nil {
			c.queue.AddRateLimited(key)
			return fmt.Errorf("error syncing item %v", key)
		}
		log.Println(key)

		c.queue.Forget(obj)
		log.Println("successfully synced")

		return nil
	}(obj)

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

	log.Printf("syncing %v %v\n", namespace, name)

	// get the foo resource with this name and namespace
	foo, err := c.fooLister.Foos(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Printf("object %v no longer exists in queue", name)
			return nil
		}
		return err
	}

	deploymentName := foo.Spec.DeploymentName
	deployment, err := c.deploymentLister.Deployments(foo.Namespace).Get(deploymentName)
	if errors.IsNotFound(err) {
		log.Println("deployment deleted, creating new deployment")
		deployment, err = c.kubeClientset.Apps().Deployments(namespace).Create(newDeployment(foo))
		if err != nil {
			return err
		}
		if err := kutil_appsv1.WaitUntilDeploymentReady(c.kubeClientset, deployment.ObjectMeta); err != nil {
			return err
		}
		log.Println("deployment created!")
	}

	if err != nil {
		return err
	}

	log.Printf("syncing: %v\n", deployment.GetName())

	if foo.Spec.Replicas != nil && *deployment.Spec.Replicas != *foo.Spec.Replicas {
		deployment, err = c.kubeClientset.Apps().Deployments(namespace).Update(newDeployment(foo))
		if err != nil {
			return err
		}
	}

	log.Println("updating foo status")
	if err := c.updateFooStatus(foo, deployment); err != nil {
		return err
	}

	return nil
}

func (c *Controller) updateFooStatus(foo *sampleV1beta1.Foo, deployment *appsv1.Deployment) error {
	fooCopy := foo.DeepCopy()
	fooCopy.Status.AvailableReplicas = deployment.Status.AvailableReplicas
	_, err := c.fooClientset.SamplecontrollerV1alpha1().Foos(fooCopy.Namespace).Update(fooCopy)
	return err
}

func newDeployment(foo *sampleV1beta1.Foo) *appsv1.Deployment {
	labels := map[string]string{
		"app":        "nginx",
		"controller": foo.Name,
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      foo.Spec.DeploymentName,
			Namespace: foo.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(foo, schema.GroupVersionKind{
					Group:   sampleV1beta1.SchemeGroupVersion.Group,
					Version: sampleV1beta1.SchemeGroupVersion.Version,
					Kind:    "Foo",
				}),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: foo.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
	}
}
