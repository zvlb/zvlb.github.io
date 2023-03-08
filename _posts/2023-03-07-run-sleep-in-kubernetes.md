---
title: "Запуск спящих подов"
categories:
  - blog
tags:
  - kubernetes
---

Довольно часто приходится запускать спящие pod'ы в Kubernetes для дебага различных компонентов, например сетевой доступности.

### Спящий pod

Для запуска спящего пода достаточно выполнить команду:
```bash
kubectl run test --image=ubuntu -n demo --restart=Never sleep 100000
```
Далее можно будет exec'нуться в pod и проивозить проверки

### Спящий pod в Deployment'e

Для того, чтобы в запущенном деплойменте вместо целевого процесса запустить в контейнере sleep, надо в `.spec.template.spec.containers` указать поле command:
```bash
      command:
        - /bin/sleep
        - "1111111"
```

Чаще всего вам не нужно менять что-то в деплойменте, чтобы протестировать какой-то функционал. Чаще всего можно решить проблему с помощью [kubectl debug](https://kubernetes.io/docs/tasks/debug/debug-application/debug-running-pod/)
{: .notice--warning}



---
На этом все. Вы великолепны ;)