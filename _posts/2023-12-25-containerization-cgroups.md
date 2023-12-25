---
title: "Контейнеризация. Cgroups"
categories:
  - blog
tags:
  - cgroups
  - kubernetes
  - container
  - containerization
toc: true
toc_label: "Содержание"
---

Для того, чтобы понимать что такое контейнеризация и как она работает, необходимо разбираться в 2 понятиях Операционной Системы (чаще всего Linix): **Cgroup** и **Namespace**. В  рамках этой статьи мы разберем что такое **Cgroup**.
{: .notice--info}

### Создание Cgroup'ы

***cgroups*** (v1 for kernel >2.6.24, v2 for >4.5) - Контрольные группы - компонент ядра линукса, который позволяет ограничивать процессы по использованию различных ресурсов, будь то ЦПУ, Оперативная память, Место на диске, Сеть и многое другое.

Поскольку все в операционной системе linux является файлами, cgroup’ы тоже управляются файлами.

Чтобы узнать, что именно мы можем ограничить с помощью cgroup, можно посмотреть в директории `/sys/fs/cgroup`.

```bash
❯ ls /sys/fs/cgroup/
blkio  cpu,cpuacct  cpuset   freezer  memory   net_cls,net_prio  perf_event  rdma     unified
cpu    cpuacct      devices  hugetlb  net_cls  net_prio          pids        systemd
```

Здесь мы видим все доступные нам типы cgroup’ов. Правильнее будет называть это контроллерами. То есть cgroup devices-контроллер или же cgroup cpu-контроллер и так далее. 

Мы можем вручную создать cgroup для необходимого контроллера, например, памяти. Просто создаем директорию с названием нашей cgroup в директории memory и в ней сразу инициализируются все необходимые каталоги для управления этой c-группой. В некоторых файлах устанавливаются различные ограничения для нашей c-группы, а в других содержится информация об актуальном состоянии группы.

```bash
❯ cd /sys/fs/cgroup/memory/
❯ mkdir test
❯ cd test/
❯ ls
cgroup.clone_children       memory.kmem.max_usage_in_bytes      memory.limit_in_bytes            memory.stat
cgroup.event_control        memory.kmem.slabinfo                memory.max_usage_in_bytes        memory.swappiness
cgroup.procs                memory.kmem.tcp.failcnt             memory.move_charge_at_immigrate  memory.usage_in_bytes
memory.failcnt              memory.kmem.tcp.limit_in_bytes      memory.numa_stat                 memory.use_hierarchy
memory.force_empty          memory.kmem.tcp.max_usage_in_bytes  memory.oom_control               notify_on_release
memory.kmem.failcnt         memory.kmem.tcp.usage_in_bytes      memory.pressure_level            tasks
memory.kmem.limit_in_bytes  memory.kmem.usage_in_bytes          memory.soft_limit_in_bytes
❯ cat memory.limit_in_bytes
9223372036854771712
❯ cat memory.usage_in_bytes
0
❯ cat cgroup.procs
```

Например, в файле ***memory.limit_in_bytes*** записывается лимит на объем памяти для процессов, связанных с этой группой, а в ***memory.usage_in_bytes*** - сколько памяти группа использует в данный момент. В файле ***cgroup.procs*** указаны процессы, связанные с нашей cgroup’ой. В нашем случае таких процессов нет, по этому файл пустой.

### Docker и Cgroup

При создании контейнера среда выполнения создает для него различные контрольные группы. Давайте это проверим.

Проводимые ниже тесты проходятся на Ubuntu 20.04, на которой установлен docker, но еще не было запущено ни одного контейнера. В Cgroup’е для работы с оперативной памятью, только созданный нами ранее test и дефолтные 2 группы - user.slice и system.slice.
{: .notice--info}

Запустим какой-нибудь контейнер, например nginx:

```bash
❯ docker run -d nginx
```

После запуска контейнера в Cgroup контроллере memory сразу создалась директория docker, в которой определяются дефолтные правила, которые будут распространятся на все наши контейнеры, и внутри появилась директория имя которой - это ID нашего контейнера

```bash
❯ ls /sys/fs/cgroup/memory/ | grep docker
docker
❯ ls /sys/fs/cgroup/memory/docker/
cgroup.clone_children                                             memory.failcnt              memory.kmem.max_usage_in_bytes  memory.kmem.tcp.max_usage_in_bytes  memory.max_usage_in_bytes        memory.pressure_level       memory.usage_in_bytes
cgroup.event_control                                              memory.force_empty          memory.kmem.slabinfo            memory.kmem.tcp.usage_in_bytes      memory.move_charge_at_immigrate  memory.soft_limit_in_bytes  memory.use_hierarchy
cgroup.procs                                                      memory.kmem.failcnt         memory.kmem.tcp.failcnt         memory.kmem.usage_in_bytes          memory.numa_stat                 memory.stat                 notify_on_release
fc574fe7b1dff38482bdd1d90e7f0f86a2e457c7f1503622e6e13c2f32e8a4e0  memory.kmem.limit_in_bytes  memory.kmem.tcp.limit_in_bytes  memory.limit_in_bytes               memory.oom_control               memory.swappiness           tasks
❯ docker ps
CONTAINER ID   IMAGE     COMMAND                  CREATED         STATUS         PORTS     NAMES
fc574fe7b1df   nginx     "/docker-entrypoint.…"   3 minutes ago   Up 3 minutes   80/tcp    affectionate_blackwell
❯ docker inspect fc574fe7b1df | grep Id
        "Id": "fc574fe7b1dff38482bdd1d90e7f0f86a2e457c7f1503622e6e13c2f32e8a4e0",
```

Если мы посмотрим на лимит использования памяти (***memory.limit_in_bytes***) для cgroup нашего контейнера, мы увидим цифру, равную доступной памяти на нашем сервере. По умолчанию объем памяти не ограничен. То есть ограничен, так как контейнер не может использовать больше ресурсов, чем есть на сервере, но этого никто не может.

```bash
❯ cat /sys/fs/cgroup/memory/docker/fc574fe7b1dff38482bdd1d90e7f0f86a2e457c7f1503622e6e13c2f32e8a4e0/memory.limit_in_bytes
9223372036854771712
```

Чтобы изменить лимит использования памяти, достаточно записать нужный нам лимит в байтах в этот файл.

Если мы запустим еще один контейнер, но ограничим ему использование оперативной памяти в 10m, по факту в эти 10Мб пропишутся в файле ***memory.limit_in_bytes*** в cgroup’е memory, созданной для нашего контейнера.

```bash
❯ docker run -d -m 10m nginx
4b47b231d39ebc7cd16a3226c721b0bee06c65095ce7774c8722824efc1bb69a
root@ubuntu20:~# cat /sys/fs/cgroup/memory/docker/4b47b231d39ebc7cd16a3226c721b0bee06c65095ce7774c8722824efc1bb69a/memory.limit_in_bytes
10485760
```

### Cgroup V1 vs Cgroup V2

Сейчас мы рассмотрели Cgroup первой версии. В 2016 году появилась вторая версия контрольных групп, которая сейчас используется во многих современных операционных системах по умолчанию. Основное различие между версиями заключается в иерархии хранения информации о cgroup’ах для разных процессов.

Если в первой версии у нас в корне директории с cgroup’ами `/sys/fs/cgroup` располагались типы наших cgroup, внутри которых уже были определены реальные cgroup’ы, связанные с процессами. Причем каждый определенный cgroup был привязан в одному типу - либо memory, либо cpu либо любой другой тип cgroup, то во второй версии в директории `/sys/fs/cgroup` расположены именно сами cgroup’ы к которым привязаны процессы, а внутри описание каждого типа контроллера и его настройка. 

То есть во второй версии каждый процесс связан с одной определенной cgroup’ой, в которой описано все ограничения для нашего процесса. Один Cgroup, в котором описано все сразу. И, соответственно, нет необходимости искать наш процесс по куче различных директорий с описанием разных типов cgroup, чтобы узнать все лимиты нашего процесса. 

![nginx-upstream-response-time-request-time](https://raw.githubusercontent.com/zvlb/zvlb.github.io/master/_posts/assets/images/cgroupv2.png)

Это дает несколько удобств. Например в первой версии Cgroup ничего не мешает нам, кроме здравого смысла конечно, определить один и тот же процесс в cgroup’ы с разным именем. Мы можем это делать, однако, во второй версии так сделать уже не получится, так как один процесс может быть только в одной cgroup’e.

Так же вторая версия cgroup дает большую гибкость для настройки иерархии для cgroup. Вложенные cgroup’ы с правами наследования прописанных настроект от родительских cgroup? Легко

Ну и многие контроллеры cgroup были переработаны. Самые большие изменения произошли с контроллером memory. В Cgroup memory-контроллер были добавлены 2 важные настройки **memory.min** и **memory.high**. **memory.min** обеспечивает, что для процессов в данной cgroup’e всегда будет доступно определенное количество оперативной памяти и никакие другие процессы, не состоящие в cgroup, не смогут ее использовать. А **memory.high** намного жестче регулирует сколько именно оперативной памяти могут использовать процессы в cgroup’е.

---
На это все. Вы прекрасны :)
