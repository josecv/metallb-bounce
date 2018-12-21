package main

import (
	"fmt"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"os"
)

func main() {
	var (
		appLabel  = os.Getenv("PLEX_APP_LABEL")
		namespace = os.Getenv("KUBE_NAMESPACE")
	)
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	corev1Client := clientset.CoreV1()
	watcher := cache.NewListWatchFromClient(corev1Client.RESTClient(), "pods", namespace,
		fields.OneTermEqualSelector("metadata.name", appLabel))
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err == nil {
				queue.Add(string(key))
			}
		},
		UpdateFunc: func(oldObj interface{}, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err == nil {
				queue.Add(string(key))
			}

		},
	}
	indexer, informer := cache.NewIndexerInformer(watcher, &v1.Pod{}, 0, handlers, cache.Indexers{})
	go informer.Run(nil)
	for {
		key, quit := queue.Get()
		if quit {
			return
		}
		func() {
			defer queue.Forget(key)
			switch k := key.(type) {
			case string:
				p, exists, err := indexer.GetByKey(k)
				if err != nil {
					fmt.Println(err)
					return
				}
				defer queue.Done(key)
				if !exists {
					return
				}
				pod := p.(v1.Pod)
				transition := transitionTime(pod)
				if transition == nil {
					return
				}
				nodeName := pod.Spec.NodeName
				speakerPod := getSpeakerPod(&corev1Client, nodeName)
				if speakerPod == nil {
					return
				}
				if !((*transition).Before(&speakerPod.CreationTimestamp)) {
					return
				}
				err = corev1Client.Pods("kube-system").Delete(speakerPod.Name, &metav1.DeleteOptions{})
				if err != nil {
					fmt.Println(err)
					return
				}
			default:
				panic(fmt.Errorf("unknown key type for %#v (%T)", key, key))
			}
		}()
	}
}

func getSpeakerPod(corev1Client *corev1.CoreV1Interface, nodeName string) *v1.Pod {
	speakerPods, err := (*corev1Client).Pods("kube-system").List(metav1.ListOptions{
		LabelSelector: "app=metallb,component=speaker",
		FieldSelector: "spec.nodeName=" + nodeName})
	if err != nil {
		fmt.Println(err)
		return nil
	}
	if speakerPods.Size() != 1 {
		fmt.Println("wtf no speaker pod found")
		return nil
	}
	return &speakerPods.Items[0]
}

func transitionTime(pod v1.Pod) *metav1.Time {
	conditions := pod.Status.DeepCopy().Conditions
	for _, condition := range conditions {
		if condition.Type == v1.PodReady && condition.Status == v1.ConditionTrue {
			return &condition.LastTransitionTime
		}
	}
	return nil
}
