---
title: "Kuberentes Admission Webhook'и. Со стороны разработки"
categories:
  - blog
tags:
  - webhook
  - golang
  - go
  - kubernetes
toc: true
toc_label: "Содержание"
---

### Общая информация

В Kubernetes уже давно есть функционал для динамичной проверки/редактирования ресурсов на этапе запроса на создание или обновления этого ресурса (`kubectl create/apply`) - [Dynamic Admission Control](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/). Функционал этот довольно прост и за последние годы очень сильно распространился, особенно на задачи, связанные с безопасностью ресурсов в кластере Kubernetes.

Работает все просто. При создании/редактировании какого-то ресурса специальный контроллер ([admission controller](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/)), который является частью `kube-apiserver`, проверяет, настроены ли для этого ресурса один из 2 сценариев: **MutatingAdmissionWebhook** или **ValidatingAdmissionWebhook**. Если настроено - выполняется webhook-запрос во внешний сервис. В зависимости от ответа внешенего сервиса запрос может быть либо отклонен, либо отредактирован (то есть в etcd запишется ресурс с "какими-то" изменениями).

### Проблематика

Во время разработки Kubernetes-операторов с функционалом Validation/Mutation Webhook вы, скорее всего, столкнетесь с 2 проблемами:

- [Operator SDK](https://sdk.operatorframework.io/docs/building-operators/golang/webhook/) для работы с Webhook'ами предполагает, что либо вы устанавливете оператор через [OLM](https://olm.operatorframework.io/), либо у вас в кластере установлен [cert-manager](https://cert-manager.io/). Если ни одно из условий не выполнено - webhook'и работать не будут. То есть разрабатываемый оператор сразу перестает быть "самодостаточным", ведь для его работы нужны какие-то особые условия.
- Вести локалькую разработку с Debug-режимом при использовании Webhook'ов - тот еще ад.

Оба этих пункта внесли свои коррективы в то, как я в итоге работаю с Webhook'ами при разработке контроллера. Об этом и напишу дальше.

### Что будем делать

Для понимания как разрабатывать Kubernetes-операторы и настраивать в них Admission Webhook'и, в рамках этой статьи мы напишем очень простой оператор, настроим ему Admission Webhook и сделаем локальную разработку хоть чуть-чуть удобной.

На всякий случай - акцент в данной статье будет именно на Validation Webhook. Для Mutation Webhook будет все +- так же. + В данной статье я не буду подробно описывать как работают Admission Webhook'и с точки зрения Kubernetes. Моя цель - показать как их можно использовать при разработке операторов и контроллеров.
{: .notice--info}

### Создаем свой Kubernetes Operator

#### Инициализация

Для наглядности работы с Kubernetes Webhook'ами, нам нужна площадка для тестов. Все просто - создаем тестовый оператор и работаем с ним. Для инициализации оператора я использую [Operator SDK](https://sdk.operatorframework.io/) версии v1.33.0

```bash
❯ operator-sdk version
operator-sdk version: "v1.33.0", commit: "542966812906456a8d67cf7284fc6410b104e118", kubernetes version: "v1.27.0", go version: "go1.21.5", GOOS: "darwin", GOARCH: "arm64"
```

Код реализуемого оператора доступен в [этом](https://github.com/zvlb/webhook-operator) репозитории.
{: .notice--info}

Инициализируем оператор:

```bash
❯ operator-sdk init --domain zvlb.github.io --repo github.com/zvlb/webhook-operator
Writing kustomize manifests for you to edit...
Writing scaffold for you to edit...
...
```

И сразу регистрирем ресурс, с которым мы будем работать и который мы будем валидировать с помощью Admission Webhook'а:

```bash
❯ operator-sdk create api --group webhook --version v1alpha1 --kind Test  --resource --controller
Writing kustomize manifests for you to edit...
Writing scaffold for you to edit...
api/v1alpha1/test_types.go
api/v1alpha1/groupversion_info.go
internal/controller/suite_test.go
internal/controller/test_controller.go
...
```

#### Определение Custom Resource'а

В файле `api/v1alpha1/test_types.go` определилась структура, для нашего ресурса. И, так как мы не хотим сейчас описывать сложной логики, сделаем так, что в Спецификации нашего ресурса будет только одно поле - `name`, и при обработке нашего ресурса все, что будет происходить, это запись сообщения со значением поля `name` в статус нашего ресурса.

Для этого в файле [`api/v1alpha1/test_types.go`](https://github.com/zvlb/webhook-operator/blob/main/api/v1alpha1/test_types.go#L27) мы должны отредактировать структуры `TestSpec` и `TestStatus` и привести их к следующему виду:

```go
type TestSpec struct {
	Name string `json:"name,omitempty"`
}

type TestStatus struct {
	Message string `json:"message,omitempty"`
}
```

Генерируем небходимые файлы для нашей структуры:

```bash
❯ make generate
```

#### Накидываем логику

При выполнении `operator-sdk create api` помимо структур для нашего CR так же сгенерировался контроллер, в котором, в методе Reconcile, описывается логика обработки ресурса.

То есть каждый раз, когда происходит какое событие с CR Test (UPDATE\CREATE\DELETE) будет вызвана функция Reconcile из пакета `internal/controller/test_controller.go`. [Опишем](https://github.com/zvlb/webhook-operator/blob/main/internal/controller/test_controller.go#L53) логику:

```go
func (r *TestReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
  // Определяем Логгер
	log := log.FromContext(ctx)

	instance := &v1alpha1.Test{}

  // Забираем ресурс для которого вызвался Reconcile из Kubernetes'а
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
    // Елси ресурс не найден это означает, что ресурс был удален и Reconcile был вызван из-за события DELETE. Обрабатывать ресурс, которого не существует, не нужно
		if api_errors.IsNotFound(err) {
			log.Info("Test instance not found. Found delete case")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log.Info("Test instance found. Found create/update case")
  
  // Записываем в поле .Status.Message нашу строку
	instance.Status.Message = fmt.Sprintf("Name is %v", instance.Spec.Name)
	if err := r.Status().Update(ctx, instance); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
```

#### Проверяем

Для проверки, что все работает правильно, нам нужен кластер Kubernetes. Я использую минималистическую инсталяцию с помощью [kind](https://kind.sigs.k8s.io/). (Для активации Admission Webhook'ов в Kind необходимо указать определенные параметры. Конфиг для старта Kind-кластера можно посмотреть [тут](https://github.com/zvlb/webhook-operator/blob/main/hack/kind-config.yaml)))

Устанавливаем CR в Kubernetes:

```bash
❯ make install
...
./webhook-operator/bin/kustomize build config/crd | kubectl apply -f -
customresourcedefinition.apiextensions.k8s.io/tests.webhook.zvlb.github.io created
```

Запускаем наш оператор:

```bash
❯ go run cmd/main.go
2024-01-31T12:47:08+02:00       INFO    controller-runtime.metrics      Metrics server is starting to listen    {"addr": ":8080"}
2024-01-31T12:47:08+02:00       INFO    setup   starting manager
2024-01-31T12:47:08+02:00       INFO    starting server {"path": "/metrics", "kind": "metrics", "addr": "[::]:8080"}
2024-01-31T12:47:08+02:00       INFO    Starting server {"kind": "health probe", "addr": "[::]:8081"}
2024-01-31T12:47:08+02:00       INFO    Starting EventSource    {"controller": "test", "controllerGroup": "webhook.zvlb.github.io", "controllerKind": "Test", "source": "kind source: *v1alpha1.Test"}
2024-01-31T12:47:08+02:00       INFO    Starting Controller     {"controller": "test", "controllerGroup": "webhook.zvlb.github.io", "controllerKind": "Test"}
2024-01-31T12:47:08+02:00       INFO    Starting workers        {"controller": "test", "controllerGroup": "webhook.zvlb.github.io", "controllerKind": "Test", "worker count": 1}
```

Создаем ресурс:

```bash
❯ cat config/samples/webhook_v1alpha1_test.yaml
apiVersion: webhook.zvlb.github.io/v1alpha1
kind: Test
metadata:
  name: test-sample
spec:
  name: zvlb

❯ kubectl create -f config/samples/webhook_v1alpha1_test.yaml
test.webhook.zvlb.github.io/test-sample created
```

В логах оператора сразу видим, что ресурс был обработан:

```log
2024-01-31T12:48:16+02:00       INFO    Test instance found. Found create/update case   {"controller": "test", "controllerGroup": "webhook.zvlb.github.io", "controllerKind": "Test", "Test": {"name":"test-sample","namespace":"default"}, "namespace": "default", "name": "test-sample", "reconcileID": "2e7f12fd-69d9-40d8-af47-d74a551439f2"}
```

И посмотрев наш CR в Kubernetes, мы увидем, что ему прописался status:

```bash
❯ kubectl get tests.webhook.zvlb.github.io test-sample -o yaml
apiVersion: webhook.zvlb.github.io/v1alpha1
kind: Test
metadata:
  creationTimestamp: "2024-01-31T10:48:16Z"
  generation: 1
  name: test-sample
  namespace: default
  resourceVersion: "1104600"
  uid: 243a9335-9de1-4943-857f-065875c5543d
spec:
  name: zvlb
status:
  message: Name is zvlb
```

То есть метод Reconcile успешно отработал и обновил наш ресурс.

### Определяем логику для Validation Webhook'а

А теперь представим, что у нас есть задача - обрабатывать этот ресурс только, если в поле `spec.name` указаны конкретные имена, а остальных - отбрасывать. Для теста определим, что возможные имена, это: *zvlb*, *bvlz* и *zbvl*. Кейс "глуповатый", но обработать его мы можем следующими путями:

- Добавить в метод Reconcile логику, которая валидирует поле `.spec.name` и записывает в поле `.status.error` - сообщение об ошибке, если она есть. Однако этот способ не очень "чистый", ведь запуск функции Reconcile происходит после того, как CR обновляется в etcd и единственный способ узнать, что с ним что-то не так - прочитать его статус и увидеть это. Получается, если наш ресурс деплоится через какуй-нибудь Continues Delivery - ресурс будет записан и пользователю сложно будет узнать, что с ним что-то не так.

- *Использовать Validation Webhook*. Eсли использовать webhook, процесс Валидации можно запустить в момент, когда пользователь создает/обновляет ресурс и если валидация завершается ошибкой - ресурс не будет записан в etcd и пользователь сразу получит ошибку. Соответственно, если используется какой-то Continues Delivery - он так же завершится ошибкой.

Использовать Validation Webhook - отличное решение для нашего кейса. Приступаем.

Если что, с 1.28 версии Kubernetes появился функицонал [Validating Admission Policy](https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/) - который идеально подходит для описываемого кейса.
{: .notice--info}

### Как работает Validation Webhook

В Kubernetes можно определить ресурс `ValidatingWebhookConfiguration`, который выглядит примерно следующим образом:
```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: test-validating-webhook-cfg
webhooks:
- admissionReviewVersions:
  - v1
  - v1beta1
  clientConfig:
    caBundle: LS0tLS1*******
    service:
      name: test-webhook-service
      namespace: defailt
      path: /validate
      port: 443
  failurePolicy: Fail
  matchPolicy: Equivalent
  name: validate.zvlb.github.io
  namespaceSelector: {}
  objectSelector: {}
  rules:
  - apiGroups:
    - webhook.zvlb.github.io
    apiVersions:
    - '*'
    operations:
    - CREATE
    - UPDATE
    resources:
    - '*'
    scope: '*'
  sideEffects: None
  timeoutSeconds: 30
```

Самые важные поля, которые мы рассмотрим, это:

- `roles`. В этом поле указывается для каких ресурсов работает этот webhook. То есть если придет запрос на *CREATE* или *UPDATE* любого ресурса из apiGroup `webhook.zvlb.github.io` - `kube-apiserver` будет обязан выполнить инструкцию, описаную в поле `clientConfig`.
- `clientConfig.service`. Тут описывается к какому Kubernetes service будет обращаться `kube-apiserver`, чтобы пройти валидацию. В зависимости от ответа этого сервиса `kube-apiserver` примет решение валидировать ресурс или нет.
- `clientConfig.caBundle`. CA-сертификат, который соответствует сертификату, по которому должно отвечать приложение, настроенное в поле `clientConfig`.

Получается при валидации ресурса присходит примерно следующее:

При успешной валидации:

![access-validation](https://raw.githubusercontent.com/zvlb/zvlb.github.io/master/_posts/assets/images/val-wh-access-validation.png)

При ошибки во время валидации:

![denied-validation](https://raw.githubusercontent.com/zvlb/zvlb.github.io/master/_posts/assets/images/val-wh-denied-validation.png)

Ничего сложного. Однако для того, чтобы все работало, наш условный "Сервис Валидации" должен:
1. Принимать запрос от *Kubernetes API Server* в определенном формате и уметь его обрабатывать
2. Отдавать результат Валидации в определенном формате, который ожидает *Kubernetes API Server*
3. Работать по протоколу https с сертификатом, о котором знает *ValidatingWebhookConfiguration*, так как ему устанавливается *caBundle* этого сертификата

Для соответствия первым 2, и частично 3, пунктам отлично подходит использование встроенного в [Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime) механизма для работы с WebHook'ами.

### Настраиваем оператор для работы с Webhook'ами

#### Готовим код

В соответствии с [документацией](https://sdk.operatorframework.io/docs/building-operators/golang/webhook/) мы можем сгенерировать все необходимое для работы с Validation Webhook:

```bash
operator-sdk create webhook --group webhook --version v1alpha1 --kind Test --defaulting --programmatic-validation
```

Однако в таком случае созданные настройки будут заточены под работу либо с [OLM](https://olm.operatorframework.io/) либо с [Cert Manager'ом](https://cert-manager.io/). Ни тот ни другой вариант меня не устраивает по причине, которую я описал выше, по этому не будем этого делать и настроим все вручную!

##### Определяем валидируюшую функцию

Поскольку мы собирается валидировать структуру Test - самое очевидное - написать для этой структуры метод, который и быдет заниматься валидацией.

Создаем файл [`api/v1alpha1/test_validate.go`](https://github.com/zvlb/webhook-operator/blob/main/api/v1alpha1/test_validate.go) и описываем валидирующий метод:

```go
func (t *Test) Validate() error {
	allowedNames := []string{"zvlb", "bvlz", "zbvl"}

	if slices.Contains(allowedNames, t.Spec.Name) {
		return nil
	}

	return fmt.Errorf("name %s not allowed", t.Spec.Name)
}
```

##### Определим Webhook Server

Для организации Webhook для работы [Controller Runtime менеджера](https://github.com/kubernetes-sigs/controller-runtime/blob/main/pkg/manager/manager.go#L55) необходимо при описании его [опций](https://github.com/kubernetes-sigs/controller-runtime/blob/main/pkg/manager/manager.go#L95) в файле `./cmd/main.go` (переменная mgr) заполнить поле WebhookServer. А в последствии зарегистрировать этот Webhook Server:

```go
mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		...
    ...
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    9443,
			CertDir: "/tmp/k8s-webhook-server/serving-certs",
		}),
	})

  mgr.GetWebhookServer().Register(
			"/validate",
			&webhook.Admission{
				Handler: &handler.Handler{},
			},
		)
```

В описанном выше коде неясно что такое `&handler.Handler{}`. Для регистрации Webhook Server'а нам нужен handler для обработки запроса. Если мы посмотрим в методе *Register*, то увидим, что Handler - это [простой интерфейс](https://github.com/kubernetes-sigs/controller-runtime/blob/main/pkg/webhook/admission/webhook.go#L98):

```go
type Handler interface {
	Handle(context.Context, Request) Response
}
```

Ничего нам не мешает описать свою реализацию этого handler'а.

Создаем файл [`internal/webhook/handler/handler.go`](https://github.com/zvlb/webhook-operator/blob/main/internal/webhook/handler/handler.go) и заполняем его логикой:

```go
type Handler struct{}

func (h *Handler) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Проверяем, что валидация вызвалась для кастом ресурса из нашей группы
	if req.AdmissionRequest.Kind.Group != "webhook.zvlb.github.io" {
		return admission.Errored(http.StatusInternalServerError, fmt.Errorf("invalid group: %s", req.AdmissionRequest.Kind.Group))
	}

  // Проверяем для какого именно ресурса была вызвана валидация. Такой подход очень удобен, на случай, если у нашей группы будет в будущем несколько ресурсов и для каждой будет неободима своя валидация
	switch res := req.AdmissionRequest.Kind.Kind; res {
	case "Test":
		object := &webhookv1alpha1.Test{}

    // Достаем наш объект из запроса
		if err := json.Unmarshal(req.Object.Raw, object); err != nil {
			return admission.Errored(http.StatusInternalServerError, fmt.Errorf("%w. %w", "cannot unmarshal", err))
		}

    // Валидируем нам объект
		if err := object.Validate(); err != nil {
			return admission.Errored(http.StatusInternalServerError, err)
		}
	}

	return admission.Allowed("")
}
```

То есть мы инициировали структуру *Handler* у которой определем метод *Handle*, чтобы соответствовать интерфейсу *Handler* из пакета [Controller Runtime](https://github.com/kubernetes-sigs/controller-runtime). Именно этот метод будет вызываться каждый раз, когда Kubernetes API Server будет вызывать наш сервер для валидации CR'ок.

##### Генерируем сетрификат для Webhook Сервера

Все, что нам осталось - это предоставить нашему серверу сертификат и ключ для работы. Причем CA этого ключа должен быть прописан в поле `caBundle` в `ValidatingWebhookConfiguration`

На самом деле сертификат, который мы собираемся предоставить Webhook Server'у, проще всего инициализировать в Secret'e и подсовывать его в Pod нашего оператора. Нам ничего не мешает написать функции для генерации сертификата и ключа и новый Reconcile для работы с этим конкретным Secret'ом

Определяем пакет [`internal/cert`](https://github.com/zvlb/webhook-operator/tree/main/internal/cert) для генерации сертификатов и ключей. Я не буду тут описывать всю логику этого пакета. Можно заглянуть и посмотреть. Единственное на что обращу внимание - если вы в рамках одного проекта/компании собираетесь писать несколько Kubernetes Operator'ов с Admission Webhook'ами - проще и правильней будет определить отдельный репозиторий для работы с сертификатами и переиспользовать его в различных операторов. Именно так и было сделано, например, для [проектов KaasOPS](https://github.com/kaasops/cert)

Определяем новый контроллер [`internal/controller/webhook_controller.go`](https://github.com/zvlb/webhook-operator/blob/main/internal/controller/webhook_controller.go) для обработки Kubernetes Secret'а с сертификатами для Webhook Server'а. Опять же, я не буду тут разбирать весь код, только опишу основну логику:
-  При страте нашего Webhook Operator'а сразу инициируется Reconcile-метод, который проверяет, что в нашем secret'е находится валидный сертификат и что в `ValidatingWebhookConfiguration` указан актуальный `caBundle`. 
- Контролируется, что сертификат в сикрете - валидный.
- Контролируется, что в `ValidatingWebhookConfiguration` в поле `caBundle` все указано верно и соответствует сертификату.
- Когда время жизни сетрификата подойдет к концу должен запуститься Reconcile, который сгенерирует новый сертификат и обновит `caBundle`.

Регистрируем новый контроллер в [`cmd/main.go`](https://github.com/zvlb/webhook-operator/blob/main/cmd/main.go#L158):

```go
if err = (&controller.WebhookReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		Namespace: installationNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Webhook")
		os.Exit(1)
	}
```

`installationNamespace` - в этой переменной необходимо указать namespace, в котором будет происталирован наш оператор. Это необходимо, чтобы webhook-контроллер знал с каким secret'ом он работает. Легко решается с помощью helm'a.

##### Проблема генерации сертификата

Есть одна проблема, которую необходимо решить. Когда мы стартуем наш оператор, стартует Webhook Server, который ожидает, что на пути `/tmp/k8s-webhook-server/serving-certs` будут 2 файла: `tls.crt` и `tls.key`. Однако при первом старте оператора - сертификатов не существует и этих файлов нет. Они там реально появятся, но только после того, как выполнится первый Reconcile нашего Webhook Controller'а. Однако функции Reconcile, по умолчанию, выполняются только после регистрации Webhook Server'а.

Проблема решается довольно просто. Нам необходимо выполнить инициализацию сертификатов 1 раз до того, как стартанет Webhook Server. Добавляем в [`cmd/main.go`](https://github.com/zvlb/webhook-operator/blob/main/cmd/main.go#L105) логику разового Reconcile сертификатов перед регистрацией сервера!

##### Запускаем и тестируем оператор

Для запуска оператора необходимо следующие манифесты:
- **CRD**. CRD нашего Test'а
- **Deployment**. В котором описывается сам Webhook Operator
- **ValidatingWebhookConfiguration**. В котором мы даем инструкции Kubernetes API Server'у, что для валидации ресурса необходимо идти в определенный сервис
- **Service** (for Webhook). Собственно сервис, в который будет ходить Kuberneres API Server для валидации и который будет вести на Webhook Server в нашем операторе (порт 9443)
- **Secret**. В котором будут хранится сертификаты. Этот Secret можно создать пустым, ведь все равно наш оператор сам насытит этот Secret нужными сертификатами
- **ServiceAccount** (так же **role** и **rolebinding**). Так как наш оператор будет ходить в Kuberenetes API.

Все манифесты доступны [ТУТ](https://github.com/zvlb/webhook-operator/tree/main/hack/manifests). Запускаем, чтобы проверить, что все работает корректно:

```bash
kubectl apply -f hack/manifests
```

Пробуем добавить CR test, который будет считаться валидным:

```bash
kubectl apply -f config/samples/webhook_v1alpha1_test.yaml
test.webhook.zvlb.github.io/test-sample created
```

Если мы попробуем добавить CR test, который не пройдет валидацию мы получим следующее:

```bash
kubectl apply -f config/samples/webhook_v1alpha1_wrong.yaml
Error from server: error when creating "config/samples/webhook_v1alpha1_wrong.yaml": admission webhook "validating-webhook.webhook.zvlb.github.io" denied the request: name lol not allowed
```

Все работает именно так, как мы и хотели. Прекрасно!

### Как настроить Debug-режим для разработки оператора с Admission Webhook'ом.

Если мы посмотрим что происходит, когда мы пытается создать CR test, то увидем примерно следующую схему:

![as-is](https://raw.githubusercontent.com/zvlb/zvlb.github.io/master/_posts/assets/images/val-wh-as-is.png)

Получается, что когда мы запустим локальный инстанс нашего оператора для разработки или дебага - Kubernetes API Server все Validation-запросы будет пытаться направить не на наш локальный инстанс Webhook Operator'а, а на его версию, запущенную в Kubernetes.

То есть для локальной разработки все, что нам надо - перехватить Validation-запрос и направить его на инстанс Webhook Operator'а, запущенный локально. Схема такая:

![to-be](https://raw.githubusercontent.com/zvlb/zvlb.github.io/master/_posts/assets/images/val-wh-to-be.png)

Нам нужно решить 2 проблемы:
- Достать сертификаты для Webhook Server'а, которые будут валидироваться настроенным `caBundle` в `ValidatingWebhookConfiguration`.
- Перенаправить запрос от Kubernetes API Server в наш локfльно запущенный Webhook Operator.

Перед началом лучше остановить Webhook Operator, запущенный в Kubernetes:

```bash
kubectl scale deployment webhook-operator --replicas 0
```

#### Настраиваем TLS-сертификаты на локально машине

Тут все просто. Когда мы запустили в Kubernetes наш оператор он уже сгенерировал необходимые сертификаты и положил их в Secret `webhook-operator-tls`. Все, что нам нужно - это достать их и положить в директорию, в которой их ждет Webhook Operator, запущенный локально:

```bash
mkdir -p /tmp/k8s-webhook-server/serving-certs
kubectl get secrets webhook-operator-tls -o jsonpath='{.data.tls\.crt}' | base64 -D > /tmp/k8s-webhook-server/serving-certs/tls.crt
kubectl get secrets webhook-operator-tls -o jsonpath='{.data.tls\.key}' | base64 -D > /tmp/k8s-webhook-server/serving-certs/tls.key
```

#### Настраиваем проксирование Validation-запроса от Kubernetes API

Поскольку я веду разработку в минималистической инсталяции Kubernetes - [kind](https://kind.sigs.k8s.io/), Kubernetes'у доступна моя локальная машина по IP адресу. Если вы хотите настроить подобное поведение на удаленном от вашей машины Kubernetes'e, то вам понадобится какой-то тунель до вашей рабочей станции.

В моем же случае, мы можем просто перенакатить сервис, в который ходит Kubernetes API server для выполнения валидации:

```bash
kubectl delete service webhook-operator-webhook-service

cat hack/local-debug/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: webhook-operator-webhook-service
spec:
  ports:
    - protocol: TCP
      port: 443
      targetPort: 9443
---
apiVersion: v1
kind: Endpoints
metadata:
  name: webhook-operator-webhook-service
subsets:
  - addresses:
      - ip: <LOCAL_IP>
    ports:
      - port: 9443

kubectl apply -f hack/local-debug/service.yaml
```

Где <LOCAL_IP> - IP моей локальной машины.

Все. Все работает. Локально запущенный Webhook Operator обрабатывает наши CR, а Kubernetes API Server перенаправляет все Admission Webhook'и на тот же самый локально запущенный оператор.

### Пару слов

1. Для демонстрации функционала использовался очень простой оператор, который был создан специально для этой статьи, однако изначально с проблемой я столкнулся при разработке [Envoy xDS Controller'а](https://github.com/kaasops/envoy-xds-controller). Если вы хотите посмотреть как я реализую функционал работы с Admission Webhook'ами на реальном примере - можно подглядеть там.

2. Не все, но очень многое, при работе с Admission Webhook'ами основано на том, как это делается в [Capsule](https://capsule.clastix.io/) от команды разработчиков [Clastix](https://clastix.io/). Возможно для углубления вам стоит заглянуть и в их код.

---
На это все. Вы прекрасны :)
