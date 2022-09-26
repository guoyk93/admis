package main

import (
	"context"
	"encoding/json"
	"github.com/guoyk93/ezadmis"
	admissionv1 "k8s.io/api/admission/v1"
	"log"
	"os"
)

func main() {
	s := ezadmis.NewWebhookServer(
		ezadmis.WebhookServerOptions{},
		func(
			ctx context.Context,
			request *admissionv1.AdmissionRequest,
			patches *[]map[string]interface{},
		) (deny string, err error) {
			buf, _ := json.MarshalIndent(request, "", "  ")
			log.Println(string(buf))
			return
		},
	)

	err := s.ListenAndServeGracefully()

	if err != nil {
		log.Println("exited with error:", err.Error())
		os.Exit(1)
	}
}
