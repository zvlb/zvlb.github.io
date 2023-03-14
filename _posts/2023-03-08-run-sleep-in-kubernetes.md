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
kubectl run test --image=ubuntu --restart=Never sleep 100000
```
Далее можно будет exec'нуться в pod и прозивдзить проверки

### Спящий pod в Deployment'e

Для того, чтобы в запущенном деплойменте вместо целевого процесса запустить sleep, надо в `.spec.template.spec.containers` указать поле command:
```bash
      command:
        - /bin/sleep
        - "1111111"
```

Чаще всего Вам не нужно менять что-то в деплойменте, чтобы протестировать какой-то функционал. Чаще всего можно решить проблему с помощью [kubectl debug](https://zvlb.github.io/blog/ephemeral-containers-kubectl-debug/)
{: .notice--warning}



---
На этом все. Вы великолепны ;)
