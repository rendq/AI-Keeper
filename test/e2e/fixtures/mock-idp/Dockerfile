FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY test/e2e/fixtures/mock-idp/ ./test/e2e/fixtures/mock-idp/
RUN CGO_ENABLED=0 go build -o /mock-idp ./test/e2e/fixtures/mock-idp

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /mock-idp /mock-idp
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/mock-idp"]
