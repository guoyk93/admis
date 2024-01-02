# ezadmis-httpcat

The tool `ezadmis-httpcat` simply prints the Admission review request.

## Installation

**Assuming we are installing to namespace `autoops`**

1. complete [RBAC Initialization for ezadmis-install](../ezadmis-install)
2. deploy YAML resources

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: install-ezadmis-httpcat-cfg
  namespace: autoops
data:
  config.json: |
    {
      "name": "ezadmis-httpcat",
      "namespace": "autoops",
      "mutating": false,
      "admissionRules": [
        {
          "apiGroups": ["*"],
          "apiVersions": ["*"],
          "resources": ["*"],
          "operations": ["*"]
        }
      ],
      "sideEffect": "None",
      "failurePolicy": "Ignore",
      "image": "yankeguo/ezadmis-httpcat"
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
      serviceAccountName: ezadmis-install
      automountServiceAccountToken: true
      containers:
        - name: install-ezadmis-httpcat
          image: yankeguo/ezadmis-install
          imagePullPolicy: Always
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
            name: install-ezadmis-httpcat-cfg
      restartPolicy: OnFailure
```

## Credits

GUO YANKE, MIT License
