---
title: "Базовые примитивы Kubernetes. Pods"
categories:
  - blog
tags:
  - pod
  - kubernetes
toc: true
toc_label: "Содержание"
---

### Общая информация

На самом деле у Pod'ов есть множество определений. Постепенно дам сразу несколько:

Основная задача Kubernetes - управлять контейнерами, однако он не управляет ими напрямую. Для управления контейнерами используется специальные программы, которые называются ***Container Runtime***. Все, что делает Kubernetes (а точнее его компонент ***kubelet***) - это взаиможествует с ***Container Runetime*** программами и передает им инструкции: 'как' и 'какой' контейнер необходимо поднять. 

Однако для того, чтобы передать какую-то инструкцию для настройки контейнера, Kubernetes'у самому надо знать что именно необходимо настроить. Именно эта информация и содержится в Pod'e. **Pod** - это сценарий или инструкция в удобочитаемом виде, которые мы передаем Kubernetes’у чтобы он знал как управлять контейнерами.

Основная цель **Pod** - предоставить логический обертку для одного или нескольких контейнеров.

Например, если ваше основное приложение написано на PHP с использованием php-fpm, для его работы вам понадобится веб-сервер, например nginx. Таким образом, ваше приложение становится полностью функционирующим, когда работают два процесса:

- php
- nginx

Именно когда эти два контейнера работают вместе, приложение является целостным и может функционировать.

**Pod** в Kubernetes - это минимальная абстракция, описывающая контейнер или контейнеры, которые составляют целостное приложение.
{: .notice--info}

## Приложение в рамках одного Pod'a

Почему так важно описывать целостное приложение в рамках одного pod'а, а не поднимать разные pod'ы, каждый с одним контейнером, и налаживать между ними связь? Здесь важную роль играет **масштабирование**.

Когда нагрузка на приложение возрастает, необходимо увеличивать количество его экземпляров, чтобы распределить нагрузку. Если наше приложение разбито на разные pod'ы, не всегда понятно, какой именно из них нужно масштабировать, и как при масштабировании, например, php-pod’ов, налаживать связность между ранее поднятыми nginx-pod’ами и новыми php-pod’ами. Если же целостное приложение находится в одном Pod'e, то увеличивая количество таких pod'ов и распределяя нагрузку, мы можем быть уверены, что проблем с производительностью не будет.

Так же для контейнеров, работающих в рамках одного Pod’а есть определенные удобства для взаимодействия:

- Такие контейнеры имеют единое сетевое пространство и могут обращаться друг к другу через localhost
- Так же такие контейнеры могут получать доступ к одним и тем же томам. Например один контейнер может генерировать какой-то кеш, а второй контейнер использовать его

### Создание Pod'а

У нас есть несколько путей, как мы можем создать pod

Первый путь - с помощью прямого запроса на создание pod’а, например с помощью kubectl.
Запустив команду:

```bash
kubectl run test --image=busybox
```

Мы создадим pod с именем test, внутри которого будет один контейнер из image busybox.

Если мы хотим более подробно описать наш pod перед созданием, мы можем описать его манифест в обычном YAML-файле, а потом, с помощью того же kubectl, применить этот манифест.

Любой ресурс Kubernetes’а, включая pod, верхнеуровнево состоит из 4 блоков:

```yaml
apiVersion:
kind:
metadata:
spec:
```

`apiVersion` - версия API, которая используется для этого ресурса. У Kubernetes’а есть несколько предустановленных ApiVersion, которые используются для базовых решений, но, по необходимости, можно устанавливать кастомные ApiVersion и ресурсы, связанные с этой api версией.  В следующих статьях мы затронем эту тему подробнее.

Pod’ы относятся к ApiVersion - **v1**

`kind` - тип объекта. В нашем случае мы хотим развернуть Pod, по этому указываем Pod. Важно, чтобы указываемый kind принадлежал именно той ApiVersion, которая указана выше. В кластере может быть несколько ресурсов с одним названием (т.е. с одним kind’ом), разделяться они будут именно по их принадлежности разным ApiVersion

`metadata` - различная верхнеуровневая информация о нашем объекте, например имя, лэйблы. Про лэйблы и зачем они нужны мы поговорим немного позднее.

`spec` - основное поле где мы описываем поведение pod’а и его контейнеров. Помимо прочего здесь указывается список контейнеров, которые относятся к этому pod’у

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  labels:
    app: test
spec:
  containers:
    - name: nginx
      image: nginx:1.23.1
```
Мы описали очень простой pod, который управляет одним контейнером nginx.

Фактически, в отделе spec Pod’ов существует множество полей, настройка которых позволяет определить различное поведение нашего Pod’а. Однако, в рамках данной статьи мы ограничимся двумя полями: **containers** и **initContainers** и то рассмотрим только базовый функционал.

### Мультиконтейнерные Pod'ы

Давайте для примера опишем такой Pod:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  labels:
    app: test
spec:
  containers:
    - name: first-container
      image: nginx:1.23.1
    - name: second-container
      image: nginx:1.23.1
```

Cейчас мы описали простой Зod, который управляет 2 nginx-контейнерами.

Теперь мы можем применить наш манифест выполнив команду:

```bash
kubectl create -f pod.yaml
```

Мы можем увидеть, что наш Pod стартовал, но не очень успешно. Один из контейнеров падает с ошибкой ***CrashLoopBackOff***. Эта ошибка означает, что контейнер постоянно падает из-за внутренней ошибки. Если мы посмотрим логи нашего контейнера:

```bash
kubectl logs my-pod -c second-container
/docker-entrypoint.sh: /docker-entrypoint.d/ is not empty, will attempt to perform configuration
/docker-entrypoint.sh: Looking for shell scripts in /docker-entrypoint.d/
/docker-entrypoint.sh: Launching /docker-entrypoint.d/10-listen-on-ipv6-by-default.sh
10-listen-on-ipv6-by-default.sh: info: Getting the checksum of /etc/nginx/conf.d/default.conf
10-listen-on-ipv6-by-default.sh: info: Enabled listen on IPv6 in /etc/nginx/conf.d/default.conf
/docker-entrypoint.sh: Launching /docker-entrypoint.d/20-envsubst-on-templates.sh
/docker-entrypoint.sh: Launching /docker-entrypoint.d/30-tune-worker-processes.sh
/docker-entrypoint.sh: Configuration complete; ready for start up
2023/03/11 14:14:22 [emerg] 1#1: bind() to 0.0.0.0:80 failed (98: Address already in use)
```

То выделяется ошибка ***Address already in use***.

Ошибка эта связана с тем, что оба наших контейнера nginx используют одно и то же сетевое пространство, и когда один контейнер стартанул, слушая 80 port, второй контейнер уже не может запуститься на этом port’у.

### Init-контейнеры

В многоконтейнерном Pod'е каждый контейнер запускает процесс, который остается активным все время, пока живет Pod. Например, в Pod’e c nginx и php-fpm, который мы рассматривали ранее, оба контейнера должны оставаться активными в любое время.

Но иногда вам может потребоваться запустить процесс, который нужно выполнить до старта основного контейнера или контейнеров. Например если перед началом работы приложения мы хотим сгенерировать и положить кеш в определенное место на файловой системе. Или же мы хотим проверить, что база данных доступна и готова принимать запросы. Или нам нужно из какого-то внешнего источника загрузить различные конфигурационные параметры для нашего приложения. Все эти задачи должны быть выполнены до того, как будет запущен основной процесс и именно для всех этих задач отлично подходит функциональность **init-контейнера**.

**initContainer** настроен в POD так же, как и все другие контейнеры, за исключением того, что он указывается внутри раздела `initContainers`, например, так:

```bash
apiVersion: v1
kind: Pod
metadata:
  name: podWithInit
  labels:
    app: init
spec:
  containers:
  - name: main-container
    image: busybox
    command: ['sh', '-c', 'echo "Приложение запущено" && sleep 3600']
  initContainers:
  - name: init-container
    image: busybox
    command: ['sh', '-c', 'echo "Init-контейнер запущен и будет работать 10 секунд" && sleep 10']
```

### Практикум

Для тренировки работы с pod'ами есть небольшой [практикум](https://killercoda.com/zvlbops/course/kubernetes-deep-dive/kubernetes-pods). Welcome

---
На это все. Вы прекрасны :)
