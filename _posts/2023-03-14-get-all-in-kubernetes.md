---
title: "Как посмотреть все ресурсы в Kubernetes?"
categories:
  - blog
tags:
  - kubernetes
toc: true
toc_label: "Содержание"
---

Быстрый ответ №1:

```bash
export NAMESPACE="<NAMESPACE_NAME>"
kubectl get all -n &NAMESPACE
```

Быстрый ответ №2:
```bash
export NAMESPACE="<NAMESPACE_NAME>"
kubectl api-resources --verbs=list --namespaced -o name  | xargs -n 1 kubectl get --show-kind --ignore-not-found --no-headers -n $NAMESPACE
```

## Почему get all не показывает ВСЕ ресурсы?

### Как работают категории ресурсов в Kubernetes

Что на самом деле происходит, когда мы выполняем команду `kubectl get all`, и как Kubernetes воспринимает этот all?

В описании каждого ресурса ([Custom Resource](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)) в Kubernetes можно указать сцециальное поле - [categories](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.19/#customresourcedefinition-v1-apiextensions-k8s-io). Это поле используется для объеденения разных custom resource в единую группу. Делая запрос на получение этой группы(категории) мы получаем все ресурсы, связанные с этой категорией.

На примере [Kyverno](https://kyverno.io) мы можем посмотреть, что в описании API своих ресурсов они используют category kyverno. (Например для ресурса [clusterpolicy](https://github.com/kyverno/kyverno/blob/v1.8.5/api/kyverno/v1/clusterpolicy_types.go#L16) или [updateRequest](https://github.com/kyverno/kyverno/blob/v1.8.5/api/kyverno/v1beta1/updaterequest_types.go#L54)). При генерации манифестов CRD для таких типов появится поле [category](https://github.com/kyverno/kyverno/blob/v1.8.5/config/crds/kyverno.io_clusterpolicies.yaml#L12).
Если у нас в кластере задеплоена Kyvern’а, то мы можем посмотреть ее CRD описание и увидеть это поле:

```bash
kubectl get crd clusterpolicies.kyverno.io -o yaml | grep "categories:" -A1
    categories:
    - kyverno

kubectl get crd updaterequest.kyverno.io -o yaml | grep "categories:" -A1
    categories:
    - kyverno
```

Получается, что наши CR объеденены в одну группу и мы можем выполнить такой запрос:

```bash
kubectl get kyverno -n kyverno
NAME                                                              BACKGROUND   VALIDATE ACTION   READY
clusterpolicy.kyverno.io/disallow-cap-audit-control               false        enforce           true
clusterpolicy.kyverno.io/disallow-cap-audit-read                  false        enforce           true
clusterpolicy.kyverno.io/disallow-cap-dac-read-search             false        enforce           true
clusterpolicy.kyverno.io/disallow-cap-net-admin                   false        enforce           true
...
...

NAME                                POLICY                 RULETYPE   RESOURCEKIND   RESOURCENAME           RESOURCENAMESPACE   STATUS      AGE
updaterequest.kyverno.io/ur-8qghh   generate-honeypot-sa   generate   Namespace      kubernetes-dashboard                       Completed   19d
```

Как мы видим, с помощью одного запроса мы получаем несколько ресурсов, которые логически связаны с одной категорией - Kyverno.

С Kyverno есть спорный момент на тему того, что запрашивая ресурсы конкретной категории (kyverno) в определенном namespace показываются ресурсы, которые не связаны с namespace (cluster scope).
Но давайте оставим этот вопрос на совести разработчиком Kyverno
{: .notice--info}

### all - тоже категория ресурсов

Как мы уже поняли, что Kyverno - это категория ресурсов, объявленных в нашем кластере Kubernetes. Так вот `all` - то же самое.

All - это категория ресурсов, в которую по умолчанию входят:

- pods
- services
- daemonsets
- deployments
- statefullsets
- replicasets

Получается, что выполняя запрос

```bash
kubectl get all
```

Мы получаем список ресурсов, связанных с этой категорией.

Помимо прочего, разрабатывая собственный операторы и контроллеры мы можем для своих CRD указывать категорию all. Тогда наши ресурсы будут показываться при запросе `kubectl get all`. Есть даже [правила](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-cli/kubectl-conventions.md#rules-for-extending-special-resource-alias---all), как это правильно делать.


## Получаем список всех ресурсов

В Kubernetes все ресурсы делятся на 2 уровня. 
Ресурсы уровня "cluster" не связаны с каким-либо namespace (например, clusterrole, clusterrolebinding и т.д.). 
Ресурсы уровня "namespace", которые всегда существуют в каком-то namespace. Если при определении такого ресурса namespace не указан, то ресурсы развернутся в namespace, который указан по умолчанию, чаще всего это default.
{: .notice--info}

Для получения всех ресурсов в namespace нам сначала надо узнать какие вообще ресурсы уровня namespace существуют в кластере:

```bash
kubectl api-resources --verbs=list --namespaced -o name 
```

Команда `kubectl api-resources` показывает все ресурсы определенные в кластере, дефолтные и кастомные. Параметр `--verbs=list` указывает, что нам нужны только те ресурсы, которые можно получить с помощью запроса get через kubectl. Параметр `--namespaced` указывает, что нам нужны ресурсы уровня namespace, ну и параметр `-o name` нужен, чтобы вывести только имена ресурсов. На выходе получится что-то вроде:

```bash
configmaps
endpoints
events
limitranges
persistentvolumeclaims
pods
podtemplates
replicationcontrollers
resourcequotas
secrets
serviceaccounts
services
controllerrevisions.apps
daemonsets.apps
deployments.apps
replicasets.apps
statefulsets.apps
...
...
```

Получив этот список, нам нужно запросить каждый ресурс в искомом namespace. Для удобства можно указать несколько параметров:
`--show-kind` - чтобы выдеть к какому kind принадлежит ресурс 

`--ignore-not-found` - не выдавать ошибку, если такой ресурс не был найден в namespace

`--no-headers` - не писать хеадер, который пишется каждый раз, когда мы запрашиваем ресурсы

В итоге получится такая команда:

```bash
export NAMESPACE="<NAMESPACE_NAME>"
kubectl api-resources --verbs=list --namespaced -o name  | xargs -n 1 kubectl get --show-kind --ignore-not-found --no-headers -n $NAMESPACE
```

Если мы хотим получить список всех ресурсов уровня кластера, немного подредактировав, можно получить следующее:

```bash
kubectl api-resources --verbs=list --namespaced=false -o name  | xargs -n 1 kubectl get --show-kind --ignore-not-found --no-headers
```

---

На этом все. Вы прекрасны :)
