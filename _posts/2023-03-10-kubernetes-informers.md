---
title: "Kubernetes informers. Как это работает"
categories:
  - blog
tags:
  - kubernetes
  - golang
toc: true
toc_label: "Содержание"
---

В этой статье мы рассмотрим, что такое informer'ы в библиотеке [client-go](https://github.com/kubernetes/client-go), зачем они нужны, и как они работают.

Так же мы напишем небольшую програмку, которая использует informer’ы, так как только реальная практика может дать понимание технологии.

Все примеры используемого кода собраны [ТУТ](https://github.com/zvlb/zvlb.github.io/blob/master/_posts/assets/golang/kubernetes-informers/main.go)
{: .notice--info}

## Зачем нужны informer’ы?
Основная задача контроллеров Kubernetes - это реагирование на изменения в контролируемых ими ресурсах или объектов. Для этого им необходимо получать информацию обо всех подобных изменениях.

Теоретически можно настроить контроллер так, чтобы он постоянно опрашивал Kubernetes API, сохранял информацию о запрашиваемых ресурсах в кеш и при следующем запросе сравнивал полученное и сохраненное. Если есть какие-то отличия, то запускается основной функционал контроллера, который гарантирует, что фактическое состояние ресурсов будет соответствовать описанию в инструкциях контроллера.

Для реализации такого подхода можно использовать Get и List, интерфесы:
```go
// Получить информацию о pod'е test в Namespace default
pod, err :=  client.CoreV1().Pods("default").
    Get(context.Background(), "test", v1.GetOptions{})

// Получить информацию о всех pod'ах во всех Namespace'ах
pods, err := client.CoreV1().Pods(corev1.NamespaceAll).
    List(context.Background(), v1.ListOptions{})
```

Однако, чтобы узнать об изменениях в pod’е test нам необходимо постоянно запрашивать о нем информацию и сравнивать ее с предыдущей. 

Можно не писать бесконечные циклы для опрашивания интересующих ресурсов и использовать интерфейс Watch:
```go
watcher, _ := client.CoreV1().Pods(corev1.NamespaceAll).Watch(context.Background(), metav1.ListOptions{})

// Вывести информацию о pod'ах и событиях связанных с ними
for event := range watcher.ResultChan() {
	pod := event.Object.(*corev1.Pod)
	fmt.Printf("Событые %v случилось с pod'ом с именем %s\n", event.Type, pod.Name)
}
```

События, которые могут происходить с pod’ами - ADDED, MODIFIED, DELETED. В зависимости от того, какое событие произошло - можно отреагировать на него соответствующим образом и обновить кеш.
Получается, если мы реализуем логику контроля за ресурсами через интерфейс watch нам надо:

1. Самостоятельно управлять кешем, дабавляя, удаляя и редактируя данные в нем
2. Использовать повторяющиеся логические структуры, если мы собираемся следить, например, за большим количеством ресурсов разного типа

Именно решения этих проблем библитека client-go предосталяет интерфейсы informers.

## Как работать с informer’ами

Например мы можем определить несложный informer для pod’ов:
```go
// Запускаем Inforner Factory. Верхнеуровневая сущность, с помощью 
// которой мы будем объявнять informer'ы
// Синхронизация кеша и реального состояния pod'ов - каждые 30 секунд
factory := informers.NewSharedInformerFactory(client, 10*time.Second)

// Объявляем Informer, который будет следить за pod'ами
podsInformer := factory.Core().V1().Pods().Informer()
```

В рамках factory мы можем объявить не один informer, где каждый будет следить за определенными сущностями в кластере Kubernetes.

Далее нам нужно запустить  factory:
```go
controlCh := make(chan struct{})
factory.Start(controlCh)
factory.WaitForCacheSync(controlCh)
```

Метод *WaitForCacheSync* будет лочить дальнейшее выполнение кода, пока кеш не будет полностью синхронизирован с информацией из Kubernetes. После этого informer’ы продожат синхронизироваться с актуальной информацией о подах, но в фоновом режиме.

Для того, чтобы достать из кеша информацию о pod’e, необходимо воспользоваться method’ом GetIndexer(). В кеше информация о pod’e по умолчанию доступна в кеше по ключу, который состоит из namespace/name
 
```go
podItem, _, _ := podsInformer.GetIndexer().GetByKey(namespace + "/" + name)
pod := podItem.(*corev1.Pod)
fmt.Printf("IP pod'а: %v", pod.Status.PodIP)
```

## Как работают informer’ы
Для понимания работы информеров можно обратиться к диаграмме от разработчиков Kubernetes:

![kubernetes-informers](https://raw.githubusercontent.com/zvlb/zvlb.github.io/master/_posts/assets/images/informers.jpeg)

**Reflector** - процесс, который при запуске делает list всех ресурсов, за которыми должен следить информер и далее подписывается с помощью Watch на изменение каждого. Каждое событие, связанное с отслеживаемыми ресурсами (например создание/удаление/обновление), будет записано в очередь Delta Fifo.

**Indexer** - создает локальный key-value кеш. Именно из этого кеша контролер будет собирать информацию о ресурсах. Особенность indexer’а в том, что мы можем генерировать разные indexer’ы с разными ключами для наших данных

## Расширяем функционал informer’ов

### Кастомные indexer’ы

Как я написал выше, мы можем расширять функционал Inexer’а наших informer’ов.
По умолчанию indexer хранит данные в key-value кеше, где ключем будет строка состоящая из namespace/podname. Однако если мы хотим доставать информацию по другим ключам, мы можем зарегестрировать новый indexer для нашего informer’а.

Допустим, мы хотим доставать информацию о pod’а по IP-адресу. Вы можете добавить новый индексатор перед запуском фабрики Informer. Новый индексатор pod будет получать экземпляр *pod и может возвращать список строковых значений, которые можно использовать в качестве ключа для такого pod’a

```go
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
```

Теперь при запуске informer’а каждый под будет проиндексирован двумя способами: по namespace/podname и по IP-адресу. И если мы заходим достать информацию о pod’е по IP (нам нужно заранее знать IP какого-нибуть pod’а):

```go
ip := "****"
items, err := podsInformer.GetIndexer().ByIndex(ByIP, ip)
```

### Обработчик событий informer’a

Инициировав и запустив informer’ы у нас есть возможность отслеживать события, которые происходят в наших indexer’ах.

После регистрации фактори и до старта информера мы можем инциировать обраточник событий:

```go
// Созданием очереди заданий и обработчика событий informer'а
queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
informerHandler := cahce.ResourceEventHandlerFuncs{
	AddFunc: func(obj interface{}) {
		pod := obj.(*corev1.Pod)
		fmt.Printf("New Pod Added to Store: %s/%s\\n", pod.Namespace, pod.Name)
		queue.Add(obj)
	},
	UpdateFunc: func(oldObj, newObj interface{}) {
		pod := newObj.(*corev1.Pod)
		fmt.Printf("Pod Updated in Store: %s/%s\\n", pod.Namespace, pod.Name)
		queue.Add(newObj)
	},
	DeleteFunc: func(obj interface{}) {
		pod := obj.(*corev1.Pod)
		fmt.Printf("Pod Deleted from Store: %s/%s\\n", pod.Namespace, pod.Name)
		queue.Add(obj)
	},
}

// Регистрируем обработчика событий informer'а
podInformer.Informer().AddEventHandler(informerHandler)
```

На всякий случай напомню, что все примеры кода из этой статьи вы можете пощупать вот [тут](https://github.com/zvlb/zvlb.github.io/blob/master/_posts/assets/golang/kubernetes-informers/main.go).

---

На это все. Вы прекрасны :)
