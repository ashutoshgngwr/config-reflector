FROM golang:1.13-alpine as builder
RUN apk add git make binutils
WORKDIR /workspace
COPY . .
RUN make build

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/build/_output/config-reflector .
USER nonroot:nonroot
ENTRYPOINT ["/config-reflector"]
