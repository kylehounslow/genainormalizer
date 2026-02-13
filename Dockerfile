FROM golang:1.26 AS builder
RUN go install go.opentelemetry.io/collector/cmd/builder@latest
WORKDIR /src
COPY go.mod go.sum builder-config.yaml ./
COPY *.go ./
RUN builder --config builder-config.yaml

FROM gcr.io/distroless/base-debian12
COPY --from=builder /src/dist/otelcol-genai /otelcol-genai
EXPOSE 4317 4318
ENTRYPOINT ["/otelcol-genai"]
CMD ["--config", "/etc/otelcol/config.yaml"]
