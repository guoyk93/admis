# ezadmis

[![Go Reference](https://pkg.go.dev/badge/github.com/yankeguo/ezadmis.svg)](https://pkg.go.dev/github.com/yankeguo/ezadmis)

Tools for building and registering Kubernetes admission webhooks

## Usage

The library `ezadmis` can reduce the complexity of writing a kubernetes admission webhook
.

All things you have to do is to implement a handler function.

```go
type WebhookHandler func(ctx context.Context, req *admissionv1.AdmissionRequest, rw WebhookResponseWriter) (err error)
```

- Parameters
  - `ctx`, context of incoming request
  - `request`, incoming `AdmissionRequest`
  - `patches`, an optional output of JSONPatch operations for mutating webhook
- Return Values
  - `deny`, if not empty, indicating this `AdmissionRequest` should be denied, and a message will be returned
  - `err`, error occurred

## Example

See [ezadmis-httpcat/main.go](cmd/ezadmis-httpcat/main.go)

## Tools

This repository provides two important tools

- [ezadmis-install](cmd/ezadmis-install)

  Reduce the complexity of installing an admission webhook

- [ezadmis-httpcat](cmd/ezadmis-httpcat)

  Print the incoming `AdmissionReview` request for debugging

## Extra Tools

See https://github.com/yankeguo/ezadmis-extra

## Credits

GUO YANKE, MIT License
