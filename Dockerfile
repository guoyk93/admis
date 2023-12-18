ARG APP_NAME=missing

FROM golang:1.21 AS builder
ARG APP_NAME
ENV CGO_ENABLED 0
WORKDIR /go/src/app
ADD . .
RUN go build -o /${APP_NAME} ./cmd/${APP_NAME}

FROM alpine:3.19
ARG APP_NAME
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /${APP_NAME} /${APP_NAME}
CMD ["/${APP_NAME}"]