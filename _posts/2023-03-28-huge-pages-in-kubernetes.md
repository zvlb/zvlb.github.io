---
title: "Huge Pages в Kubernetes?"
categories:
  - blog
tags:
  - kubernetes
toc: true
toc_label: "Содержание"
---

Начиная с релиза 1.26 Kubernetes'а в stable перешла новая фича - контроль [HugePages](https://kubernetes.io/docs/tasks/manage-hugepages/scheduling-hugepages/). В данной статье мы рассмотрим что это такое, зачем оно нужно и как с этим работать.


## Что такое HugePages?

Когда какой-то процесс использует в своей работе оперативную память (далее - RAM), процессор (далее - CPU) помечает, что этот кусок RAM используется нашим процессом.
По умолчанию на современных операционных системах CPU фрагментирует RAM кусочками по 4 КилоБайта. Эти 4КБ и называются pages. Получается, что вся используемая RAM на операционной системе порезана на кусочки по 4КБ и только CPU знает какой кусок какому процессу принадлежит.

Если наш процесс использует много RAM, тогда CPU тратит много ресурсов, чтобы определить какие именно Pages принадлежат нашему процессу и где именно находится нужная информация.

Например, если наш процесс использует 1Гб RAM, то это 262114 pages. Если для хранения 1 pages CPU использует 8 байт, тогда для хранения всех этих Pages и поиска по ним потребуется 2МБ (262114 * 8).

Однако операционные системы поддерживают возможность увеличения размера Pages с 4КБ, что позволит CPU обрабатывать меньше Pages. Именно такие Pages, которые больше, чем 4КБ и называются Huge Pages. (Каждый отдельный Huge Page станет больше по размеру, но их общее чисто будет меньше)


## Как использовать HugePages в Kubernetes?

Первым делом необходимо настроить наши worker-ноды. Мы можем включить Huge Pages в размером 2МБ или 1Гб на каждый ноде отредактировав соответствующие файлы:
```bash
# 1024 - это количество pages с размером 2048kB (2МБ) разрешенные на системе
echo "1024" > /sys/devices/system/node/node0/hugepages/hugepages-2048kB/nr_hugepages

# 5 - это количество pages с размером 1048576kB (1ГБ) разрешенные на системе
echo "5" > /sys/devices/system/node/node0/hugepages/hugepages-1048576kB/nr_hugepages
```

После этого ребутаем Kubelet
```bash
systemctl restart kubelet
```

Теперь если мы посмотрим подробности о нашей ноде, мы увидем, что нам теперь доступны Huge Pages:
```bash
kubectl describe node node01
Name:               node01
...
...
Capacity:
  ...
  hugepages-1Gi:      5Gi
  hugepages-2Mi:      2Gi
  ...
Allocatable:
  ...
  hugepages-1Gi:      5Gi
  hugepages-2Mi:      2Gi
  ...
```

Поскольку нам доступно 1024 pages с размером 2МБ, то в сумме нам доступно 2Гб места (1024 * 2Мб)
{: .notice--info}

Теперь мы можем создавать pod'ы, выделяя им Huge Pages:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: huge-pages-example
spec:
  containers:
  - name: example
    image: fedora:latest
    command:
    - sleep
    - inf
    volumeMounts:
    - mountPath: /hugepages-2Mi
      name: hugepage-2mi
    - mountPath: /hugepages-1Gi
      name: hugepage-1gi
    resources:
      limits:
        hugepages-2Mi: 100Mi
        hugepages-1Gi: 2Gi
        memory: 100Mi
      requests:
        memory: 100Mi
  volumes:
  - name: hugepage-2mi
    emptyDir:
      medium: HugePages-2Mi
  - name: hugepage-1gi
    emptyDir:
      medium: HugePages-1Gi
```

Если в нашем Pod планируется использовать Huge Pages только одного типа (либо по 2МБ, либо по 1ГБ) можно немного упростить:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: example
spec:
  containers:
  - name: example
    image: fedora:latest
    command:
    - sleep
    - inf
    volumeMounts:
    - mountPath: /hugepages
      name: hugepage
    resources:
      requests:
        hugepages-2Mi: 1Gi
      limits:
        hugepages-2Mi: 1Gi
  volumes:
  - name: hugepage
    emptyDir:
      medium: HugePages

```

---

На этом все. Вы прекрасны :)
