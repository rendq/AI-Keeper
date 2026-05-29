FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY test/e2e/fixtures/mock-llm/ ./test/e2e/fixtures/mock-llm/
RUN CGO_ENABLED=0 go build -o /mock-llm ./test/e2e/fixtures/mock-llm

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /mock-llm /mock-llm
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/mock-llm"]
