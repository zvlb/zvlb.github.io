---
title: "Эфимерные контейнеры. kubectl debug"
categories:
  - blog
tags:
  - kubernetes
toc: true
toc_label: "Содержание"
---

В этой статье мы рассмотрим, что [эфимерные контейнеры](https://kubernetes.io/docs/concepts/workloads/pods/ephemeral-containers/). Зачем они нужны и как они работают.

Так же мы научимся пользоваться утилитой [kubectl debug](https://kubernetes.io/docs/tasks/debug/debug-application/debug-running-pod/#ephemeral-container) для отладки контейнеров в pod'e.

## Что такое Ephemeral Containers?

Вместе с релизом [Kubernetes v1.25](https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.25.md#ephemeral-containers-graduate-to-stable) в stable перешла одна очень значимая функциональность - Эфимерные контейнеры [****Ephemeral Containers****], которые могут помочь сильно снизить горизонт атаки для злоумышленника, и расширить возможности при анализе ошибок в pod’ах.

Pod - это низкоуровневая и нередактируемая сущность в Kubernetes’e. После создании pod’а вы не можете изменить/удалить/добавить контенеры внутри пода. Если же необходио что-то поменять в контейнерах - необходимо убить существующий pod и поднять новый с нужными настройками. (Именно это и происходит, когда редактируется блок containers в Deployment’e)

Эфемерные контенеры же можно запустить внутри живого pod’а в той же зоне видимости, что и обычные контейнеры. У них будут общие сеть, cgroup’ы, древо процессов и т.д.

Эфемерные контейнеры сильно урезаны в возможностях и функциональности. Например у них отсутствует возможность настройки внешнего порта. Или же они никогда не будут перезапущены.

Так же эти контейнеры создаются с помощью специального обработчика в Kubernetes API - ephemeralcontainers. Соответственно в Pod.Spec нельзя указать ephemeralcontainers, то есть создать такой контейнер с помощью kubectl apply/create/edit не получится.

Как сейчас происходит работа с различными ошибками в pod’ах:

1. `kubectl exec` - в рабочий pod, в котором уже происходят манипуляции и, собственно, debug
2. Пересоздания пода с инструментаци debug’а и последующий `kubectl exec`

В первом случае у нас в кластере pod на постоянной основе работает с shell/bash, что сильно упрощает жизнь злоумышленнику, который смог попасть внутрь пода.

Во втором случае довольно много телодвижений надо совершить, чтобы иметь возможность провалиться в pod, чтобы начать дебажить. Причем нет 100% уверенности, что при удалении-создании нового pod’а ошибка в нем воспроизведется. Так же необходимо сделить, чтобы Debug-режим был выключен, после проведения работ.

Однако создавая эфимерные контейнеры с pod’e, где запущен основной контейнер, мы можем получить bash/shell и различные инструменты для дебага не перезапуская pod!

## kubectl debug

Для создания и управлениями эфимерными контейнерами была разработана специальная утилита - **kubectl debug**. Давайте посмотрим, как это работает.

Создадим pod, который будем дебажить:
```bash
kubectl run nginx --image nginx
```

Перед тем, как начать debug - нам нужно узнать имя контейнера, который мы хотим инспектировать:
```bash
kubectl get pod nginx -o jsonpath="{.spec.containers[*].name}"
nginx
```

В нашем случае в нашем pod’e только один контейнер - nginx. К нему мы и будет присоеденять эфимерный контейнер:
```bash
export PODNAME=nginx 
export CONTAINERNAME=nginx 
kubectl debug -it $PODNAME --image busybox --target=$CONTAINERNAME -- bash
Targeting container "nginx". If you don't see processes from this container it may be because the container runtime doesn't support this feature.
Defaulting debug container name to debugger-m942k.
If you don't see a command prompt, try pressing enter.
/ #
```

Вы сразу провалитесь в дебаг-контейнер и вам будет доступно:
```bash
// Файловая система контейнера-пациента
~ ls /proc/1/root

// Все процессы контейнера-пациента
~ ps aux

// Сеть контейнера-пациенты
~ curl localhost
```

Если мы с другого терминала посмотрим спеку нашего пода, мы увидем там следующее:
```yaml
kubectl get pod nginx -o yaml
apiVersion: v1
kind: Pod
metadata:
  labels:
    run: nginx
  name: nginx
spec:
  ...
  ephemeralContainers:
  - command:
    - sh
    image: busybox
    imagePullPolicy: Always
    name: debugger-m942k
    resources: {}
    stdin: true
    targetContainerName: nginx
    terminationMessagePath: /dev/termination-log
    terminationMessagePolicy: File
    tty: true
...
status:
  ...
  ephemeralContainerStatuses:
  - containerID: containerd://afac217b1833f38c214efa460adfffeb94c270b67c2cf7f73d260da28ec0a103
    image: docker.io/library/busybox:latest
    imageID: docker.io/library/busybox@sha256:c118f538365369207c12e5794c3cbfb7b042d950af590ae6c287ede74f29b7d4
    lastState: {}
    name: debugger-m942k
    ready: false
    restartCount: 0
    state:
      running:
        startedAt: "2023-03-11T08:31:09Z"
```

Как только вы закончите работу в debug-контейнере и выйдете из него, в status.ephemeralContainers[*].lastState пропишется:
```yaml
    state:
      terminated:
        containerID: containerd://afac217b1833f38c214efa460adfffeb94c270b67c2cf7f73d260da28ec0a103
        exitCode: 130
        finishedAt: "2023-03-11T08:40:06Z"
        reason: Error
        startedAt: "2023-03-11T08:31:09Z"
```

### Запуск debug’а на копии pod'а

Если вдруг вы не хотите затрагивать основной контейнер, который выполняет полезную нагрузку, вы можете запустить копию pod’а используя агрумент `--copy-to=`, например:
```bash
kubectl debug -it $PODNAME --image busybox --copy-to=nginx-debug -- sh
Defaulting debug container name to debugger-76v7n.
If you don't see a command prompt, try pressing enter.
/ #
```

В таком случае нам не надо использовать аргумент `--target=`, так как наш pod полностью копируется.

Теперь вы можете увидеть, что появился новый pod - nginx-debug и у него 2 контейнера - основной nginx и эфимерный busybox:
```bash
kubectl get pod
NAME          READY   STATUS    RESTARTS   AGE
nginx         1/1     Running   0          20m
nginx-debug   2/2     Running   0          14s
```

## Образ для Debug’а

В примерах выше для debug-контейнера я использовать busybox, однако его функционала, чаще всего, недостаточно для проверки работоспособности приложения. 

Чаще всего для того или иного процесса debag’a вам придется билдить или искать подходящие образы. Например для проверки проблем с DNS, вам понадобится image c dig’ом, для тестирования сетевой доступность - с curl’ом и т.д.

Можно упростить себе жизнь используя images от [Nixery](https://nixery.dev).
Nixery позволяет запросить image с необходимым набором инструментов, просто перечислив их в URL.

Например скачивая образ: `nixery.dev/shell/git/htop/dig` вы получите образ, в котормо установлены все пречисленные программы.

Соответственно мы можем использовать такой image для нашего debug-контейнера:
```bash
kubectl debug -it nginx --image nixery.dev/shell/git/htop/dig --target nginx -- bash
```

---

На этом все. Вы прекрасны :)
