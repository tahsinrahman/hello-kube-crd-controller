package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	clientset "github.com/tahsinrahman/hello-kube-crd-controller/pkg/client/clientset/versioned"
	"github.com/tahsinrahman/hello-kube-crd-controller/pkg/controller"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	fooInformers "github.com/tahsinrahman/hello-kube-crd-controller/pkg/client/informers/externalversions"
	kubeInformers "k8s.io/client-go/informers"
)

func main() {

	masterURL := ""
	kubeconfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "config")

	config, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfigPath)
	if err != nil {
		log.Fatalf("error building kubeconfig: %v", err)
	}

	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal("error building kubernetes clientset: %v", err)
	}

	fooClient, err := clientset.NewForConfig(config)
	if err != nil {
		log.Fatal("error building foo clientset: %v", err)
	}

	kubeInformerFactory := kubeInformers.NewSharedInformerFactory(kubeClient, time.Minute*10)
	fooInformerFactory := fooInformers.NewSharedInformerFactory(fooClient, time.Minute*10)

	crdController := controller.NewController(
		kubeClient,
		fooClient,
		kubeInformerFactory.Apps().V1().Deployments(),
		fooInformerFactory.Samplecontroller().V1alpha1().Foos(),
	)

	stopCh := make(chan struct{})

	kubeInformerFactory.Start(stopCh)
	fooInformerFactory.Start(stopCh)

	if err := crdController.Run(2, stopCh); err != nil {
		log.Println("error running controller: %v", err)
	}

}
