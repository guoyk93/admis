package ezadmis

import (
	"bytes"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	PathServiceAccountNamespace = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

// InClusterNamespace determine current namespace from in-cluster environment
func InClusterNamespace() (string, error) {
	buf, err := os.ReadFile(PathServiceAccountNamespace)
	return string(bytes.TrimSpace(buf)), err
}

// InClusterClient create a kubernetes client from in-cluster environment
func InClusterClient() (client *kubernetes.Clientset, err error) {
	var cfg *rest.Config
	if cfg, err = rest.InClusterConfig(); err != nil {
		return
	}
	if client, err = kubernetes.NewForConfig(cfg); err != nil {
		return
	}
	return
}
