package ezadmis

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	admissionv1 "k8s.io/api/admission/v1"
)

// WebhookHandler function to modify incoming kubernetes resource;
// use 'patches' to output JSON patch modifications;
// return non-empty `deny` value to deny this modification
type WebhookHandler func(ctx context.Context, request *admissionv1.AdmissionRequest, patches *[]map[string]interface{}) (deny string, err error)

// WrapWebhookHandlerOptions options for wrapping WebhookHandler
type WrapWebhookHandlerOptions struct {
	Debug bool
}

// WrapWebhookHandler wrap WebhookHandler to http.HandlerFunc
func WrapWebhookHandler(opts WrapWebhookHandlerOptions, handler WebhookHandler) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		// automatically error returning
		var err error
		defer func() {
			if err == nil {
				return
			}
			log.Println("failed to handle mutating admission review:", err.Error())
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}()

		// decode request
		var reqReview admissionv1.AdmissionReview
		if err = json.NewDecoder(req.Body).Decode(&reqReview); err != nil {
			err = errors.New("failed to decode incoming AdmissionReview: " + err.Error())
			return
		}

		if opts.Debug {
			log.Println("Request:")
			raw, _ := json.Marshal(&reqReview)
			log.Println(string(raw))
		}

		// execute handler
		var (
			retDeny    string
			retPatches []map[string]interface{}
		)

		if retDeny, err = handler(req.Context(), reqReview.Request, &retPatches); err != nil {
			err = errors.New("failed to execute WebhookHandler: " + err.Error())
			return
		}

		if opts.Debug {
			log.Println("Patches:")
			if len(retPatches) == 0 {
				log.Println("--- NONE ---")
			} else {
				raw, _ := json.MarshalIndent(retPatches, "", "  ")
				log.Println(string(raw))
			}
			log.Println("Deny:", retDeny)
		}

		// build response
		var resReview admissionv1.AdmissionReview

		{
			var patch []byte
			var patchType *admissionv1.PatchType
			if len(retPatches) != 0 {
				if patch, err = json.Marshal(retPatches); err != nil {
					err = errors.New("failed to marshal WebhookHandler patches: " + err.Error())
					return
				}
				patchType = new(admissionv1.PatchType)
				*patchType = admissionv1.PatchTypeJSONPatch
			}

			var status *metav1.Status
			if retDeny != "" {
				status = &metav1.Status{
					Status:  metav1.StatusFailure,
					Message: retDeny,
					Reason:  metav1.StatusReasonBadRequest,
				}
			}

			resReview = admissionv1.AdmissionReview{
				TypeMeta: reqReview.TypeMeta,
				Response: &admissionv1.AdmissionResponse{
					UID:       reqReview.Request.UID,
					Allowed:   retDeny == "",
					Result:    status,
					Patch:     patch,
					PatchType: patchType,
				},
			}
		}

		// send response
		var buf []byte
		if buf, err = json.Marshal(resReview); err != nil {
			err = errors.New("failed to marshal outgoing AdmissionReview: " + err.Error())
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("Content-Length", strconv.Itoa(len(buf)))
		_, _ = rw.Write(buf)
	}
}

// WebhookServer webhook server abstraction
type WebhookServer interface {
	// ListenAndServe wraps internal http.Server#ListenAndServeTLS()
	ListenAndServe() error

	// ListenAndServeGracefully ListenAndServe() with signal handling, perfect for using
	// inside main() as the only component
	ListenAndServeGracefully() error

	// Shutdown wraps internal http.Server#Shutdown()
	Shutdown(ctx context.Context) error
}

// WebhookServerOptions options for WebhookServer
type WebhookServerOptions struct {
	Port     int
	CertFile string
	KeyFile  string
	Debug    bool
}

var (
	webhookServerDefaultOptions = WebhookServerOptions{
		Port:     443,
		CertFile: "/admission-server/tls.crt",
		KeyFile:  "/admission-server/tls.key",
	}
)

// WebhookServerDefaultOptions returns default options for WebhookServer
func WebhookServerDefaultOptions() WebhookServerOptions {
	return webhookServerDefaultOptions
}

type webhookServer struct {
	opts WebhookServerOptions
	s    *http.Server
}

func (w *webhookServer) ListenAndServe() error {
	return w.s.ListenAndServeTLS(w.opts.CertFile, w.opts.KeyFile)
}

func (w *webhookServer) ListenAndServeGracefully() (err error) {
	chErr := make(chan error, 1)
	chSig := make(chan os.Signal, 1)
	signal.Notify(chSig, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		chErr <- w.ListenAndServe()
	}()

	select {
	case err = <-chErr:
	case sig := <-chSig:
		log.Println("signal caught:", sig.String())
		err = w.Shutdown(context.Background())
	}
	return
}

func (w *webhookServer) Shutdown(ctx context.Context) error {
	return w.s.Shutdown(ctx)
}

// NewWebhookServer create a WebhookServer
func NewWebhookServer(opts WebhookServerOptions, handler WebhookHandler) WebhookServer {
	dfo := WebhookServerDefaultOptions()
	if opts.Port == 0 {
		opts.Port = dfo.Port
	}
	if opts.CertFile == "" {
		opts.CertFile = dfo.CertFile
	}
	if opts.KeyFile == "" {
		opts.KeyFile = dfo.KeyFile
	}
	return &webhookServer{
		opts: opts,
		s: &http.Server{
			Addr: ":" + strconv.Itoa(opts.Port),
			Handler: WrapWebhookHandler(
				WrapWebhookHandlerOptions{
					Debug: opts.Debug,
				},
				handler,
			),
		},
	}
}
