package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/creasty/defaults"
	"github.com/go-playground/validator/v10"
	"github.com/yankeguo/ezadmis/pkg/x509util"
	"github.com/yankeguo/rg"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	ezadmisInstallCA = "ezadmis-install-ca"

	serviceAccountNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

type Options struct {
	Name      string `json:"name" validate:"required"`
	Namespace string `json:"namespace"`

	Mutating       bool                                         `json:"mutating"`
	AdmissionRules []admissionregistrationv1.RuleWithOperations `json:"admissionRules" validate:"required"`
	SideEffect     admissionregistrationv1.SideEffectClass      `json:"sideEffect" default:"Unknown" validate:"required"`
	FailurePolicy  admissionregistrationv1.FailurePolicyType    `json:"failurePolicy" default:"Fail" validate:"required"`

	Image            string                        `json:"image" validate:"required"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets"`
	ImagePullPolicy  corev1.PullPolicy             `json:"imagePullPolicy" default:"Always"`
	Affinity         *corev1.Affinity              `json:"affinity"`
	NodeSelector     map[string]string             `json:"nodeSelector"`
	ServiceAccount   string                        `json:"serviceAccount"`
	Port             int                           `json:"port" default:"443" validate:"required"`
	Env              []corev1.EnvVar               `json:"env"`
	Command          []string                      `json:"command"`
	Args             []string                      `json:"args"`
	TLSCrtPath       string                        `json:"tlsCrtPath" default:"/admission-server/tls.crt" validate:"required"`
	TLSKeyPath       string                        `json:"tlsKeyPath" default:"/admission-server/tls.key" validate:"required"`
	Volumes          []corev1.Volume               `json:"volumes"`
	VolumeMounts     []corev1.VolumeMount          `json:"volumeMounts"`
	Containers       []corev1.Container            `json:"containers"`
	Resources        corev1.ResourceRequirements   `json:"resources"`
	InitContainers   []corev1.Container            `json:"initContainers"`
}

func detectNamespace() (string, error) {
	buf, err := os.ReadFile(serviceAccountNamespacePath)
	return string(bytes.TrimSpace(buf)), err
}

func createClient() (client *kubernetes.Clientset, err error) {
	var cfg *rest.Config

	if cfg, err = rest.InClusterConfig(); err != nil {
		if cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			nil,
		).ClientConfig(); err != nil {
			return
		}
	}

	return kubernetes.NewForConfig(cfg)
}

type resourceAPI[T any] interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*T, error)
	Create(ctx context.Context, obj *T, opts metav1.CreateOptions) (*T, error)
}

func detectResourceName(v any) string {
	type withMetadata struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	var obj withMetadata
	if buf, err := json.Marshal(v); err == nil {
		_ = json.Unmarshal(buf, &obj)
	}
	return obj.Metadata.Name
}

func ensureResource[T any](ctx context.Context, api resourceAPI[T], obj *T) (out *T, err error) {
	name := detectResourceName(obj)

	if name == "" {
		err = errors.New("ensure: missing metadata.name")
		return
	}

	if out, err = api.Get(ctx, name, metav1.GetOptions{}); err != nil {
		if kerrors.IsNotFound(err) {
			out, err = api.Create(ctx, obj, metav1.CreateOptions{})
		}
	}

	return
}

func ensureCertificate(
	ctx context.Context,
	api resourceAPI[corev1.Secret],
	name string,
	opts x509util.GenerateOptions,
) (secret *corev1.Secret, res x509util.PEMPair, err error) {
	if secret, err = api.Get(ctx, name, metav1.GetOptions{}); err != nil {
		if kerrors.IsNotFound(err) {
			if res, err = x509util.Generate(opts); err != nil {
				return
			}

			if secret, err = api.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Type: corev1.SecretTypeTLS,
				StringData: map[string]string{
					corev1.TLSCertKey:       string(res.Crt),
					corev1.TLSPrivateKeyKey: string(res.Key),
				},
			}, metav1.CreateOptions{}); err != nil {
				return
			}
		}
	} else {
		res.Crt, res.Key = secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey]

		if res.IsZero() {
			err = fmt.Errorf("missing key: %s or %s", corev1.TLSCertKey, corev1.TLSPrivateKeyKey)
			return
		}
	}
	return
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ltime | log.Lmsgprefix)

	var err error

	defer func() {
		if err == nil {
			return
		}
		log.Println("exited with error:", err.Error())
		os.Exit(1)
	}()

	defer rg.Guard(&err)

	var argConf string

	flag.StringVar(&argConf, "conf", "config.json", "config file")
	flag.Parse()

	bufConf := rg.Must(os.ReadFile(argConf))

	var opts Options

	rg.Must0(json.Unmarshal(bufConf, &opts))
	rg.Must0(defaults.Set(&opts))
	rg.Must0(validator.New().Struct(&opts))

	client := rg.Must(createClient())

	// determine namespace
	if opts.Namespace == "" {
		if opts.Namespace, err = detectNamespace(); err != nil {
			err = nil
		}
	}
	if opts.Namespace == "" {
		opts.Namespace = metav1.NamespaceDefault
	}

	log.Println("bootstrapping admission webhook", opts.Name, "in namespace:", opts.Namespace)

	ctx := context.Background()

	_, ca := rg.Must2(
		ensureCertificate(
			ctx,
			client.CoreV1().Secrets(opts.Namespace),
			ezadmisInstallCA,
			x509util.GenerateOptions{
				IsCA:  true,
				Names: []string{"EZAdmisInstall root ca"},
			},
		),
	)

	log.Println("ca certificate ensured:", string(ca.Crt))

	secretName := opts.Name + "-crt"

	_, leaf := rg.Must2(
		ensureCertificate(
			ctx,
			client.CoreV1().Secrets(opts.Namespace),
			secretName,
			x509util.GenerateOptions{
				Parent: ca,
				Names: []string{
					opts.Name,
					opts.Name + "." + opts.Namespace,
					opts.Name + "." + opts.Namespace + ".svc",
					opts.Name + "." + opts.Namespace + ".svc.cluster",
					opts.Name + "." + opts.Namespace + ".svc.cluster.local",
				},
			},
		),
	)

	log.Println("leaf certificate ensured:", string(leaf.Crt))

	workloadSelector := map[string]string{
		"k8s-app": opts.Name,
	}

	rg.Must(ensureResource[corev1.Service](
		ctx,
		client.CoreV1().Services(opts.Namespace),
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: opts.Name,
			},
			Spec: corev1.ServiceSpec{
				Selector: workloadSelector,
				Type:     corev1.ServiceTypeClusterIP,
				Ports: []corev1.ServicePort{
					{
						Name:       "https",
						Protocol:   corev1.ProtocolTCP,
						Port:       int32(opts.Port),
						TargetPort: intstr.FromInt(opts.Port),
					},
				},
			},
		},
	))

	log.Println("service ensured:", opts.Name)

	const volumeNameTLS = "vol-ezadmis-tls"

	rg.Must(ensureResource[appsv1.StatefulSet](
		ctx,
		client.AppsV1().StatefulSets(opts.Namespace),
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: opts.Name,
			},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: workloadSelector,
				},
				ServiceName: opts.Name,
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: workloadSelector,
					},
					Spec: corev1.PodSpec{
						ImagePullSecrets: opts.ImagePullSecrets,
						InitContainers:   opts.InitContainers,
						Affinity:         opts.Affinity,
						NodeSelector:     opts.NodeSelector,
						Containers: append([]corev1.Container{
							{
								Name:            opts.Name,
								Image:           opts.Image,
								ImagePullPolicy: opts.ImagePullPolicy,
								Command:         opts.Command,
								Args:            opts.Args,
								Env:             opts.Env,
								Ports: []corev1.ContainerPort{
									{
										Name:          "https",
										Protocol:      corev1.ProtocolTCP,
										ContainerPort: int32(opts.Port),
									},
								},
								VolumeMounts: append([]corev1.VolumeMount{
									{
										Name:      volumeNameTLS,
										SubPath:   corev1.TLSCertKey,
										MountPath: opts.TLSCrtPath,
									},
									{
										Name:      volumeNameTLS,
										SubPath:   corev1.TLSPrivateKeyKey,
										MountPath: opts.TLSKeyPath,
									},
								}, opts.VolumeMounts...),
								Resources: opts.Resources,
							},
						}, opts.Containers...),
						ServiceAccountName: opts.ServiceAccount,
						Volumes: append([]corev1.Volume{
							{
								Name: volumeNameTLS,
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: secretName,
									},
								},
							},
						}, opts.Volumes...),
					},
				},
			},
		}),
	)

	log.Println("statefulset ensured:", opts.Name)

	time.Sleep(time.Second * 10)

	qualifiedName := opts.Namespace + "-" + opts.Name

	if opts.Mutating {
		rg.Must(ensureResource[admissionregistrationv1.MutatingWebhookConfiguration](
			ctx,
			client.AdmissionregistrationV1().MutatingWebhookConfigurations(),
			&admissionregistrationv1.MutatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: qualifiedName,
				},
				Webhooks: []admissionregistrationv1.MutatingWebhook{
					{
						Name: qualifiedName + ".ezadmis-install.guoyk93.github.io",
						ClientConfig: admissionregistrationv1.WebhookClientConfig{
							CABundle: ca.Crt,
							Service: &admissionregistrationv1.ServiceReference{
								Namespace: opts.Namespace,
								Name:      opts.Name,
							},
						},
						Rules:                   opts.AdmissionRules,
						SideEffects:             &opts.SideEffect,
						FailurePolicy:           &opts.FailurePolicy,
						AdmissionReviewVersions: []string{"v1"},
					},
				},
			},
		))
	} else {
		rg.Must(ensureResource[admissionregistrationv1.ValidatingWebhookConfiguration](
			ctx,
			client.AdmissionregistrationV1().ValidatingWebhookConfigurations(),
			&admissionregistrationv1.ValidatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: qualifiedName,
				},
				Webhooks: []admissionregistrationv1.ValidatingWebhook{
					{
						Name: qualifiedName + ".ezadmis-install.guoyk93.github.io",
						ClientConfig: admissionregistrationv1.WebhookClientConfig{
							CABundle: ca.Crt,
							Service: &admissionregistrationv1.ServiceReference{
								Namespace: opts.Namespace,
								Name:      opts.Name,
							},
						},
						Rules:                   opts.AdmissionRules,
						SideEffects:             &opts.SideEffect,
						FailurePolicy:           &opts.FailurePolicy,
						AdmissionReviewVersions: []string{"v1"},
					},
				},
			},
		))
	}

	log.Println("validating/mutating webhook ensured:", opts.Name)
}
