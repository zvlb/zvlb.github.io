---
title: "Nginx. upstream_response_time больше, чем request_time"
categories:
  - blog
tags:
  - opensource
  - nginx
toc: true
toc_label: "Содержание"
---

При анализе логов nginx, можно столкнуться с ситуацией, когда у обрабатываемого запроса значение `request_time` меньше, чем `upstream_response_time`, что теоретически невозможно. В данной статье разберем, почему это происходит и как решить эту проблему.

### Немного вводных данных

При анализе логов Nginx есть несколько критичных метрик. `Request_time` и `upstream_response_time` - одни из таких метрик.

**Request_time** - время прошедшее между первым байтом, прочитанным с клиента при отправке запроса и последним байтом, который был отправлен клиенту. То есть это время - это сумарное время обработки запроса, пришедшего на nginx.

**Upstream_response_time** - время, которое nginx ожидал обработку запроса от "бекенда", куда запрос был перенаправлен. Причем, если в upstream в nginx указано несколько серверов и при обработке запроса первый не успел ответить за какое-то время или ответил с ошибкой, при определенных настройках, запрос будет перенаправлен на другой сервер, указанный в upstream и тогда в метрике `upstream_response_time` будет через запятую указано несколько значений времени. За какое время ответил первый сервер, второй и так далее.

То есть по факту `request_time` - это `upstream_response_time` + сколько времени ушло у самого nginx на обработку запроса. Схематически это можно изобразить так:

![nginx-upstream-response-time-request-time](https://raw.githubusercontent.com/zvlb/zvlb.github.io/master/_posts/assets/images/nginx-upstream-response-time-request-time.png)

Из этого следуюет, что `request_time` всегда должен быть больше, чем `upstream_response_time`.

### Проблема

При анализе логов можно столкнуться с ситуацией, когда `upstream_response_time` больше, чем `request_time` и эта разница не должна превыщать 4 милисекунды.
```json
{
  "request_time": "0.062",
  "upstream_response_time": "0.064",
}
{
  "request_time": "0.078",
  "upstream_response_time": "0.080",
}
{
  "request_time": "0.002",
  "upstream_response_time": "0.004",
}
```

### Причина

На самом деле все довольно просто. При расчете времени для `upstream_response_time` и `request_time` используются разные функции, которые предоставляет Linux.
Для расчета `request_time` используется [gettimeofday](https://man7.org/linux/man-pages/man2/gettimeofday.2.html), а для `upstream_response_time` - [clock_gettime(CLOCK_MONOTONIC_COARSE)](https://linux.die.net/man/2/clock_gettime). И как раз у `clock_gettime` есть особенность, заключаящаяся в том, что он зависит от настройки ядра - `CONFIG_HZ`, в которой настраивается с какой частотой прерывается таймер. По умолчанию, на многих операционных системах этот параметр установлен в значение 250, что означает, что таймер будет прерываться каждые 4 милисекунды. А это значит, что при подсчете `upstream_response_time` ему может добавиться от 0 до 4 милисекунд, если в операционной системе `CONFIG_HZ` установлен в 250. По этому и получается, что в логах обработки запроса `upstream_response_time` может быть больше, чем `request_time`.

Проверить значение `CONFIG_HZ` на вашей ОС можно с помощью команды:
```bash
cat /boot/config-5.4.0-150-generic | grep CONFIG_HZ
# CONFIG_HZ_PERIODIC is not set
# CONFIG_HZ_100 is not set
CONFIG_HZ_250=y
# CONFIG_HZ_300 is not set
# CONFIG_HZ_1000 is not set
CONFIG_HZ=250
```
Файл `config-*-generic` может отличаться в зависимости от используемой версии ядра.

---
На этом настройка завершена. Вы великолепны ;)
