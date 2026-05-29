FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY test/e2e/fixtures/mock-feishu/ ./test/e2e/fixtures/mock-feishu/
RUN CGO_ENABLED=0 go build -o /mock-feishu ./test/e2e/fixtures/mock-feishu

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /mock-feishu /mock-feishu
USER nonroot:nonroot
EXPOSE 8082
ENTRYPOINT ["/mock-feishu"]
