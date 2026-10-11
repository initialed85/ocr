# Tesseract OCR service. No GPU, so it schedules on any amd64 node.
#
# Static binary (no cgo) -> the runtime image only needs the tesseract CLI and
# eng tessdata.
FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /ocr .

FROM debian:13-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends tesseract-ocr ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /ocr /usr/local/bin/ocr

ENV PORT=8080
EXPOSE 8080
USER nobody
ENTRYPOINT ["/usr/local/bin/ocr"]
