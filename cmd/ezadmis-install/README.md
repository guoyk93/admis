# ezadmis-install

The tool `ezadmis-install` can reduce the complexity of installing an admission webhook to kubernetes cluster.

## Installation

```shell
go install github.com/guoyk93/ezadmis/cmd/ezadmis-install@latest
```

*ensure your `$GOPATH/bin` is a part of your `$PATH`*

## Configuration

```json5
{
  // name, name of your admission webhook
  // this will be the name of Service, StatefulSet, etc.
  "name": "ezadmis-httpcat",
  // namespace, in which namespace your webhook will be installed
  "namespace": "autoops",
  // mutating, whether this is a mutating webhook
  "mutating": false,
  // admissionRules, what kubernetes operations should be reviewed by this webhook
  // check https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/#configure-admission-webhooks-on-the-fly for syntax
  // in this example, "CREATE" operation of resource "pods" in core api group will be reviewed.
  "admissionRules": [
    {
      "apiGroups": [
        ""
      ],
      "apiVersions": [
        "*"
      ],
      "resources": [
        "pods"
      ],
      "operations": [
        "CREATE"
      ],
      "scope": "Namespaced"
    }
  ],
  // sideEffect, side effect of this webhook
  // should be one of 'Unknown', 'None', 'Some' or 'NoneOnDryRun'
  "sideEffect": "None",
  // failurePolicy, whether failure of calling this webhook should block the original request.
  // should be one of 'Ignore' or 'Fail'
  "failurePolicy": "Ignore",
  // image, image of your admission webhook
  "image": "guoyk/ezadmis-httpcat",
  // serviceAccount, the service account your webhook will use
  "serviceAccount": "default",
  // port, on which port your webhook is listening
  "port": 443,
  // env, any extra environment variables your webhook need
  "env": [
    {
      name: "aaa",
      value: "bbb"
    }
  ],
  // mountPath
  "mountPath": {
    // where the auto generated tls secret should be mounted
    // default to /admission-server/tls.crt and /admission-server/tls.key
    // these are default values of 'WebhookServerOptions' of 'ezadmis' library
    "tlsCrt": "/admission-server/tls.crt",
    "tlsKey": "/admission-server/tls.key",
  }
}
```

## Usage

```shell
ezadmis-install -conf config.json
```

Just one-run command and `ezadmis-install` will do the following steps:

1. create ca `ezadmis-install-ca`
2. create leaf certificate for your webhook
3. create `Service` for your webhook
4. create `StatefulSet` for your webhook
5. create corresponding `MutatingWebhookRegistration` or `ValidatingWebhookRegistration` for your webhook

## In-Cluster Usage

`ezadmis-install` can execute in-cluster, as long as `RBAC` is set up correctly.

Here is an example for running `ezadmis-install` as `Job` in namespace `autoops`:

```yaml
# ServiceAccount and RBAC
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ezadmis-install
  namespace: autoops
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ezadmis-install
rules:
  - apiGroups: [ "" ]
    resources: [ "secrets", "services" ]
    verbs: [ "get", "create" ]
  - apiGroups: [ "apps" ]
    resources: [ "statefulsets" ]
    verbs: [ "get", "create" ]
  - apiGroups: [ "admissionregistration.k8s.io" ]
    resources: [ "mutatingwebhookconfigurations", "validatingwebhookconfigurations" ]
    verbs: [ "get", "create" ]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ezadmis-install
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: ezadmis-install
subjects:
  - kind: ServiceAccount
    name: ezadmis-install
    namespace: autoops
---
# Configuration
apiVersion: v1
kind: ConfigMap
metadata:
  name: ezadmis-httpcat-install-cfg
  namespace: autoops
data:
  config.json: |
    {
      "name": "ezadmis-httpcat",
      "namespace": "autoops",
      "mutating": false,
      "admissionRules": [
        {
          "apiGroups": [
            ""
          ],
          "apiVersions": [
            "*"
          ],
          "resources": [
            "pods"
          ],
          "operations": [
            "CREATE"
          ],
          "scope": "Namespaced"
        }
      ],
      "sideEffect": "None",
      "failurePolicy": "Ignore",
      "image": "guoyk/ezadmis-httpcat",
      "serviceAccount": "default",
      "port": 443,
      "env": [
        {
          "name": "aaa",
          "value": "bbb"
        }
      ],
      "mountPath": {
        "tlsCrt": "/admission-server/tls.crt",
        "tlsKey": "/admission-server/tls.key"
      }
}
---
# Job
apiVersion: batch/v1
kind: Job
metadata:
  name: install-ezadmis-httpcat
  namespace: autoops
spec:
  template:
    spec:
      serviceAccount: ezadmis-install
      containers:
        - name: install-ezadmis-httpcat
          image: guoyk/ezadmis-install
          args:
            - /ezadmis-install
            - -conf
            - /config.json
          volumeMounts:
            - name: vol-cfg
              mountPath: /config.json
              subPath: config.json
      volumes:
        - name: vol-cfg
          configMap:
            name: ezadmis-httpcat-install-cfg
      restartPolicy: OnFailure
```

## Donation

View <https://guoyk93.github.io/#donation>

## Credits

Guo Y.K., MIT License
