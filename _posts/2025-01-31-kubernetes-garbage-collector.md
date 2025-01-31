---
title: "Kuberentes Garbage Collector. Как он работает"
categories:
  - blog
tags:
  - golang
  - go
  - kubernetes
toc: true
toc_label: "Содержание"
---

В рамках этой статьи мы рассмотрим как работает **Garbage Collector** в Kubernetes. Данная статья была написана, когда мне пришлось разбираться в производительности **Garbage Collector**, а чтобы понять как его можно ускорить, сначала надо понять как он работает. Для точного понимания мы будем запускать **Garbage Collector** локально в дебаг режиме, чтобы максимально ясно видеть как он работает и что делает.


### Основное про Garbage Collector

В кластере Kubernetes у некоторых объектов есть *Родитель*, или же *Владелец* (то есть *Owner*). Классическая связка зависимых объектов, это *Pod*-*ReplicaSet*-*Deployment*. То есть у *Pod*'a Owner - это *ReplicaSet*, с которым он связан. У *ReplicaSet* Owner - *Deployment*. 

Давайте развернем простой *Deployment* и посмотрим у кого какой Owner прописался у созданных объектов

```bash
❯ kubectl create deployment demo --image nginx
❯ kubectl get pod demo-677cfb9d49-kk5rd -o jsonpath='{.metadata.ownerReferences}' | jq
[
  {
    "apiVersion": "apps/v1",
    "blockOwnerDeletion": true,
    "controller": true,
    "kind": "ReplicaSet",
    "name": "demo-677cfb9d49",
    "uid": "80cb3139-6dec-49f1-a104-4ed1321539df"
  }
]

❯ kubectl get replicasets.apps demo-677cfb9d49 -o jsonpath='{.metadata.ownerReferences}' | jq
[
  {
    "apiVersion": "apps/v1",
    "blockOwnerDeletion": true,
    "controller": true,
    "kind": "Deployment",
    "name": "demo",
    "uid": "ccc4728a-41f8-4757-8605-2337382a1c89"
  }
]
```

Зачем это нужно? Все довольно просто. Данная иерархия позволяет Kubernetes удалять лишние ресурсы в кластере в автоматическом режиме. Например, когда мы удаляем *Deployment* обычной командой:

```bash
❯ kubectl delete deployment demo
```

Все что происходит - это удаляется *Deployment*. Однако когда мы удаляем *Deployment* мы так же ожидаем, что удалятся все его *ReplicaSet*'ы и *Pod*'ы. Именно этим и занимается **gc**. Он видит, что удалился *Deployment* и фоном запускает процесс убийства его зависимых ресурсов (то есть *ReplicaSet*'ов). Далее он видит, что удалились *ReplicaSet*'ы и удаляет уже его зависимые объекты - *Pod*'ы. 

Если вы хотите переопределить логику удаления ресурса и "ждать" пока все его зависимые ресурсы удалятся, тогда можно запустить команду **delete** с флагом **--cascade=foreground**. В таком случае команда удаления заблокируется и будет ожидать удаления всех зависимых объектов 
{: .notice--info}

### Инициализируем среду

Начнем с того, что **Garbage Collector** - это один из компонентов [Kubernetes Controller Manager](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-controller-manager/)'a. А это значит, что мы довольно просто можем отключить его или же включить Controller Manager, в котором будет работать только **GC**. Делается это с помощью аргумента [controllers](https://kubernetes.io/docs/reference/command-line-tools-reference/kube-controller-manager/). Давайте настроим среду, где мы будем запускать gc локально.

Для этого нам сначала нужно создать кластер Kubernetes, в котором будет выключен **GC**. Для быстрых локальных Kuberentes я использую [KIND](https://kind.sigs.k8s.io/), который инициализирует ноды с помощью kubeadm, и, соответственно, мы можем сконфигурировать каждый компонент Kubernetes Control Plane с помощью [InitConfiguration](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta3/#kubeadm-k8s-io-v1beta3-InitConfiguration) или [ClusterConfiguration](https://kubernetes.io/docs/reference/config-api/kubeadm-config.v1beta3/#kubeadm-k8s-io-v1beta3-ClusterConfiguration).

```bash
❯ cat <<EOF | kind create cluster --name without-gc --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    controllerManager:
      extraArgs:
        "controllers": "*,-garbage-collector-controller"
EOF
```

Проверим, что у нашего кластера не работает **GC**. Для этого создадим в кластере *Deployment*, потом удалим его и убедимся, что его *ReplicaSet*'ы и *Pod*'ы оставись жить.

```bash
❯ kubectl create deployment demo --image nginx
deployment.apps/demo created

❯ sleep 5
❯ kubectl get pods
NAME          READY  STATUS       RESTARTS  AGE
demo-6f84b74674-xr7v2  0/1   ContainerCreating  0     6s

❯ kubectl delete deployment demo
deployment.apps/demo deleted

❯ sleep 60
❯ kubectl get rs
NAME       DESIRED  CURRENT  READY  AGE
demo-6f84b74674  1     1     1    75s

❯ k get po
NAME          READY  STATUS  RESTARTS  AGE
demo-6f84b74674-xr7v2  1/1   Running  0     77s
```

Все как мы и ожидали. Теперь мы можем склонировать себе репозиторий [Kubernetes](https://github.com/kubernetes/kubernetes) и запустить Controller Manager c **gc** локально. В зависимости от используемой вами IDE, процесс может немного отличаться, но основное, что нужно сделать, это запусить `./cmd/kube-controller-manager/controller-manager.go` с арументами:

```
--controllers=garbage-collector-controller
--kubeconfig=/Users/zvlb/.kube/config
--v=2
--leader-elect=false
--profiling=true
--authorization-always-allow-paths=/metrics,/debug/controllers/garbage-collector-controller/graph,/debug/pprof/*
```

Пройдемся по аргументам:
- **controllers**. Если в Controller Manager, который работает в kind мы отключили все, кроме gc, то в локально запускаемом Controller Manager'e мы запускаем ТОЛЬКО **gc**
- **kubeconfig**. Поскольку мы запускаем Controller Manager локально - ему нужно передать где ему искать kubeconfig с данными для подключения к Kuberentes
- **v**. Контролируем уровень логирования
- **leader-elect**. Мы запускаем один инстанс Controller Manager'а, которому не с кем конфликтовать и определять лидера. Отключаем этот функционал.
- **profiling**. Включаем go-профилировщик
- **authorization-always-allow-paths**. Выключаем необходимость авторизировать запросы на дебажный функционал

Как только мы его запустим, мы увидим, что *ReplicaSet*'ы и *Pod*'ы убитого ранее *Deployment*'а успешно удалились!

### Как работает Garbage Collector

Запустив локальный **gc** мы можем, наконец-то, посмотреть как он работает и что делает. В этой части будет много отлылок к коду и последовательный анализ того, что происходит.

Версия Kubernetes, код которой мы будем разглядывать код - v1.32.1
{: .notice--info}

Первым делом **Garbage Collector** стартует. За это отвечает функция [*startGarbageCollectorController*](https://github.com/kubernetes/kubernetes/blob/v1.32.1/cmd/kube-controller-manager/app/core.go#L609C6-L609C37), в которой в goroutine запускается 2 основных процесса: [Run](https://github.com/kubernetes/kubernetes/blob/v1.32.1/pkg/controller/garbagecollector/garbagecollector.go#L132) и [Sync](https://github.com/kubernetes/kubernetes/blob/v1.32.1/pkg/controller/garbagecollector/garbagecollector.go#L175). Run при запуске ждет когда выполнится логика в Sync'e, по этому сначала рассмотрим его и последовательно разберем. 

Вообще у Sunc'a одна задача - следить, что для каждого ресурса в Kubernetes запущен свой *Informer* (в **Garbage Collector**'e *informer*'ы обернуты в дополнительную сущность - [monitor](https://github.com/kubernetes/kubernetes/blob/v1.32.1/pkg/controller/garbagecollector/graph_builder.go#L121)), чтобы отслеживать события **ADD**/**UPDATE**/**DELETE** на каждый объект в Kuberentes. Основная задача Sync'a - это регистрировать monitor на каждый CR и следить, появились ли новые CR, чтобы для них тоже зарегестрировать monitor. Происходит это слудующим образом:
1. Sync [собирает](https://github.com/kubernetes/kubernetes/blob/v1.32.1/pkg/controller/garbagecollector/garbagecollector.go#L181) информацию о всех CR, которые есть в кластере
2. [Сравнивает](https://github.com/kubernetes/kubernetes/blob/v1.32.1/pkg/controller/garbagecollector/garbagecollector.go#L198) полученный счисок ресурсов (newResources) с тем, который был получен при прошлом исполнении Sync'a (oldResources). Если они одинаковые - Sync завершается
3. Если находятся какие-то отличия, запускается [resync](https://github.com/kubernetes/kubernetes/blob/v1.32.1/pkg/controller/garbagecollector/garbagecollector.go#L123) всех monitor'ов. Если monitor's существовали ранее - ничего не изменится, однако, если появился новый ресурс (Custom Resource), он запустит для него monitor.

Это вся логика Sync'a) Однако надо немного заострить внимание на том, что такое monitor.

Monitor состоит из *Informer Controller*'a и *Informer Store*'a. Для Controller'a регистрируется [handler](https://github.com/kubernetes/kubernetes/blob/v1.32.1/pkg/controller/garbagecollector/graph_builder.go#L189), который вызывает определенную логику в зависимости от события, которое произошло с объектом (**ADD**/**UPDATE**/**DELETE**), за которым следит *Informer*. По факту на любое событие регистрируется event, который обрабатывает метод [processGraphChanges](https://github.com/kubernetes/kubernetes/blob/v1.32.1/pkg/controller/garbagecollector/graph_builder.go#L678), который в зависимости от события сделает 1 из 2:
- Если событие **ADD** или **UPDATE** анализируемый объект будет добавлен в граф, куда пропишутся связи объекта с его *Owner*'ом.
- Если событие **DELETE** объект будет удален из графа и в очередь *attemptToDelete* будет передана информация о всех дочерних объектах, которые были определены ранее в графе.

Подробнее про то, как работают Kuberentes Informer'ы можно почитать в [другой моей статье](https://zvlb.github.io/blog/kubernetes-informers/)
{: .notice--info}

Очередь *attemptToDelete* обрабатывается воркерами, которые [инциализируется](https://github.com/kubernetes/kubernetes/blob/v1.32.1/pkg/controller/garbagecollector/garbagecollector.go#L160) в Run функции. По факту все, что попадет в эту очередь будет отправлено на удаление из Kubernetes.

То есть если удалить *Deployment* произойдет следующее:
1. Informer Controller, который настроен на *Deployment*'ы увидит событие **DELETE** и передаст объект в очедедь *graphChanges*
2. *processGraphChanges* увидит, что в очереди *graphChanges* новое событие на удаление и отправит в очередь *attemptToDelete* все дочерние объекты нашего *Deployment*'a, то есть его *ReplicaSet*'ы
3. Informer Controller, который настроен на *ReplicaSet*'ы увидит событие **DELETE** и передаст объект в очедедь *graphChanges*
4. *processGraphChanges* увидит, что в очереди *graphChanges* новое событие на удаление и отправит в очередь *attemptToDelete* все дочерние объекты нашего *ReplicaSet*'a, то есть его *Pod*'ы
5. Informer Controller, который настроен на *Pod*'ы увидит событие **DELETE** и передаст объект в очедедь *graphChanges*
6. *processGraphChanges* увидит, что в очереди *graphChanges* новое событие на удаление и отправит в очередь *attemptToDelete* все дочерние объекты нашего *Pod*'a, но поскольку таких нет - ничего не произойдет

Вот так и удаляются объекты из Kubernetes!

### Немного тонкостей при работе Garbage Collector

#### Graph Debug

Запуская **gc** и включив его профилировку (агрумент `--profiling=true`) и выключив авторизацию на путь `/debug/controllers/garbage-collector-controller/graph` (аргумент `--authorization-always-allow-paths=/debug/controllers/garbage-collector-controller/graph`) мы можем в любой момент времени достать из **gc** весь сформированный граф и проанализировать связи, которые в нем сформировались.

Заупстив локалько Controller Manager c gc по пути `https://localhost:10257/debug/controllers/garbage-collector-controller/graph` - вы увидите граф. Однако в таком формате ничего не понятно. Можно выгрузить граф и перевести его в формат SVG

```
 curl -k https://localhost:10257/debug/controllers/garbage-collector-controller/graph | dot -T svg -o gc.svg
```

Сам файл:

![gitHubFlow](https://raw.githubusercontent.com/zvlb/zvlb.github.io/master/_posts/assets/images/gc-graph.png)

#### Долгий прогрев

Если у вас много, действительно МНОГО, ресурсов в кластере Kuberentes, может случится неприятная ситуация. Garbage Collector не может начать удалять ресурсы из Kubernetes, пока не сформировался Граф. При формировании графа gc берет ВСЕ объекты из кластера Kuberentes, на что может уйти время. А это значит, что при рестарте gc, или при смене лидера возможна ситуация, когда в Kubernetes не удаляются дочерние ресурсы из-за того, что Garbage Collector занят формированием графа, а не удалением объектов. 

На самом деле ситуая рестарта или смены лидера у Controller Manager'a - редкая и данная ситуация проблем не вызывает, что если вам необходимо улучшить производительность, можео подкрутить следующие параметры `--kube-api-qps`, `--kube-api-burst` и `--concurrent-gc-syncs`

---
На это все. Вы прекрасны :)
