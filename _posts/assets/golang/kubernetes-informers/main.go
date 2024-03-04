package main

// Для старта не забудьте выполнить go mod init *** и go mod tidy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

func main() {
	// Узнаем расположение Домашней директории
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Не удалось определить домашнюю директорию: %v\n", err)
		os.Exit(1)
	}

	// Узнаем путь к Kubeconfig
	kubeConfigPath := filepath.Join(userHomeDir, ".kube", "config")

	// Инициализация конфигурации Kubernetes
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		fmt.Printf("error getting Kubernetes config: %v\n", err)
		os.Exit(1)
	}

	// Созданем клиент для работы с Kubernetes API
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// fmt.Println("Тестирование работы интерфейса Get")
	// testGet(clientset)

	// fmt.Println("Тестирование работы интерейса List")
	// testList(clientset)

	// fmt.Println("Тестирование работы интерфейса Watch")
	// testWatch(clientset)

	fmt.Println("Тестирование работы интерфейса Watch c ingress'ами")
	testWatchIngress(clientset)

	// fmt.Println("Тестирование работы informer'а")
	// testInformer(clientset)

	// fmt.Println("Тестирование кастомного indexer'а")
	// testInformerIndexer(clientset)

	// fmt.Println("Тестирование обработчика informer'а")
	// testInformerHandler(clientset)

}

func testGet(client *kubernetes.Clientset) {
	// Получить информацию о pod'е test в Namespace default
	pod, err := client.CoreV1().Pods("default").Get(context.Background(), "test", v1.GetOptions{})
	if err != nil {
		fmt.Printf("Не удалось получить информацию о pod'e %s в namespace %s. Ошибка: %+v\n", "test", "default", err)
		return
	}

	fmt.Printf("Pod name: %+v\n", pod.Name)

	return
}

func testList(client *kubernetes.Clientset) {
	// Получить информацию о всех pod'ах во всех Namespace'ах
	pods, err := client.CoreV1().Pods(corev1.NamespaceAll).List(context.Background(), v1.ListOptions{})
	if err != nil {
		fmt.Printf("Не удалось все pod'ы в кластере Ошибка: %+v\n", err)
	}

	fmt.Printf("Количетсво pod'ов в кластере: %+v\n", len(pods.Items))

	return
}

func testWatch(client *kubernetes.Clientset) {
	// Подписаться на информацию о всех pod'ах во всех Namespace'ах
	watcher, err := client.CoreV1().Pods(corev1.NamespaceAll).Watch(context.Background(), v1.ListOptions{})
	if err != nil {
		fmt.Printf("Не подписаться на все pod'ы в кластере Ошибка: %+v\n", err)
	}

	// Вывести информацию о pod'ах и событиях связанных с ними
	for event := range watcher.ResultChan() {
		pod := event.Object.(*corev1.Pod)
		fmt.Printf("Событые %v случилось с pod'ом с именем %s\n", event.Type, pod.Name)
	}

	return
}

func testWatchIngress(client *kubernetes.Clientset) {
	// Подписаться на информацию о всех ingress'ах во всех Namespace'ах
	watcher, err := client.NetworkingV1().Ingresses(corev1.NamespaceAll).Watch(context.Background(), v1.ListOptions{})
	if err != nil {
		fmt.Printf("Не подписаться на все pod'ы в кластере Ошибка: %+v\n", err)
	}

	// Вывести информацию о pod'ах и событиях связанных с ними
	for event := range watcher.ResultChan() {
		ing := event.Object.(*networkingv1.Ingress)
		fmt.Printf("Namespace: %s, Ingress: %s, Event: %s\n", ing.Namespace, ing.Name, event.Type)
	}

	return
}

func testInformer(client *kubernetes.Clientset) {
	// Запускаем Inforner Factory. Верхнеуровневая сущность, с помощью
	// которой мы будем объявнять informer'ы
	// Синхронизация кеша и реального состояния pod'ов - каждые 30 секунд
	factory := informers.NewSharedInformerFactory(client, 10*time.Second)

	// Объявляем Informer, который будет следить за pod'ами
	podsInformer := factory.Core().V1().Pods().Informer()

	controlCh := make(chan struct{})
	factory.Start(controlCh)
	factory.WaitForCacheSync(controlCh)

	podItem, _, _ := podsInformer.GetIndexer().GetByKey("default" + "/" + "test")
	pod := podItem.(*corev1.Pod)
	fmt.Printf("IP pod'а: %v", pod.Status.PodIP)

	return
}

func testInformerIndexer(client *kubernetes.Clientset) {
	// Запускаем Inforner Factory. Верхнеуровневая сущность, с помощью
	// которой мы будем объявнять informer'ы
	// Синхронизация кеша и реального состояния pod'ов - каждые 30 секунд
	factory := informers.NewSharedInformerFactory(client, 10*time.Second)

	// Объявляем Informer, который будет следить за pod'ами
	podsInformer := factory.Core().V1().Pods().Informer()

	const ByIP = "IndexByIP"
	podsInformer.AddIndexers(map[string]cache.IndexFunc{
		ByIP: func(obj interface{}) ([]string, error) {
			var ips []string
			for _, ip := range obj.(*corev1.Pod).Status.PodIPs {
				ips = append(ips, ip.IP)
			}
			return ips, nil
		},
	})

	controlCh := make(chan struct{})
	factory.Start(controlCh)
	factory.WaitForCacheSync(controlCh)

	ip := "10.17.0.79"
	items, _ := podsInformer.GetIndexer().ByIndex(ByIP, ip)

	for _, pod := range items {
		fmt.Printf("Pod с IP: %s, находится в namespace %+v, а имя его %s\n", ip, pod.(*corev1.Pod).ObjectMeta.Namespace, pod.(*corev1.Pod).ObjectMeta.Name)
	}
	return
}

func testInformerHandler(client *kubernetes.Clientset) {
	// Запускаем Inforner Factory. Верхнеуровневая сущность, с помощью
	// которой мы будем объявнять informer'ы
	// Синхронизация кеша и реального состояния pod'ов - каждые 30 секунд
	factory := informers.NewSharedInformerFactory(client, 10*time.Second)

	// Объявляем Informer, который будет следить за pod'ами
	podsInformer := factory.Core().V1().Pods().Informer()

	// Созданием очереди заданий и обработчика событий informer'а
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	informerHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			fmt.Printf("Добавлен новый pod в indexer: %s/%s\n", pod.Namespace, pod.Name)
			queue.Add(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			pod := newObj.(*corev1.Pod)
			fmt.Printf("Обновлен pod в indexer'e: %s/%s\n", pod.Namespace, pod.Name)
			queue.Add(newObj)
		},
		DeleteFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			fmt.Printf("Удален pod из indexer'a: %s/%s\n", pod.Namespace, pod.Name)
			queue.Add(obj)
		},
	}

	// Регистрируем обработчика событий informer'а
	podsInformer.AddEventHandler(informerHandler)

	controlCh := make(chan struct{})
	factory.Start(controlCh)
	factory.WaitForCacheSync(controlCh)

	time.Sleep(30 * time.Second)

	return
}
